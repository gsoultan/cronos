package sql_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
A second factor is a second factor, or it is a lie.

The portal has shown a two-factor enrolment wizard since before there was a
server to enrol against, and it accepted any six digits. An account marked as
protected by a secret nobody holds is worse than one with no second factor: the
owner picks a weaker password because they believe something else is guarding it.

Two failures make it decorative, and both are here. Enrolment that is never
proved, and a code that works twice.
*/

// The store's clock, which these tests compute codes against.
var storeNow = time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)

func enrolled(t *testing.T, s *store.Store, id string) string {
	t.Helper()
	person(t, s, id)

	secret, err := identity.NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Enrol(context.Background(), id, secret, "Authenticator app"); err != nil {
		t.Fatal(err)
	}
	return secret
}

func codeAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := identity.TOTPCode(secret, at)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

/*
An enrolment nobody proved is not protection.

The whole failure of the wizard this replaces. Offering a QR code and trusting
that it was scanned marks an account as protected by a secret that may exist
only on the server.
*/
func TestAnUnconfirmedEnrolmentDoesNotProtectAnything(t *testing.T) {
	s := open(t)
	enrolled(t, s, "usr_ada")

	if s.Protected(context.Background(), "usr_ada") {
		t.Fatal("an unproved enrolment counts as a second factor")
	}
	if _, err := s.FactorOf(context.Background(), "usr_ada"); !errors.Is(err, identity.ErrNoFactor) {
		t.Fatalf("the account page would show it: %v", err)
	}
}

func TestConfirmingWithARealCodeTurnsItOn(t *testing.T) {
	s := open(t)
	secret := enrolled(t, s, "usr_ada")

	if err := s.Confirm(context.Background(), "usr_ada", codeAt(t, secret, storeNow)); err != nil {
		t.Fatal(err)
	}
	if !s.Protected(context.Background(), "usr_ada") {
		t.Fatal("a confirmed factor does not protect the account")
	}

	f, err := s.FactorOf(context.Background(), "usr_ada")
	if err != nil {
		t.Fatal(err)
	}
	if f.Label != "Authenticator app" || f.AddedAt.IsZero() {
		t.Fatalf("%+v", f)
	}
}

// A wrong code does not confirm. This is the assertion the old wizard failed:
// it accepted any six digits, so every enrolment "succeeded".
func TestAWrongCodeDoesNotConfirm(t *testing.T) {
	s := open(t)
	secret := enrolled(t, s, "usr_ada")

	right := codeAt(t, secret, storeNow)
	for _, wrong := range []string{"000000", "123456", "999999"} {
		if wrong == right {
			continue
		}
		if err := s.Confirm(context.Background(), "usr_ada", wrong); !errors.Is(err, identity.ErrBadCode) {
			t.Fatalf("%q confirmed the enrolment: %v", wrong, err)
		}
	}
	if s.Protected(context.Background(), "usr_ada") {
		t.Fatal("a wrong code turned protection on")
	}
}

// And a code from somebody else's secret does not, which is what "checked
// against the stored secret" means in practice.
func TestAnotherSecretsCodeDoesNotConfirm(t *testing.T) {
	s := open(t)
	enrolled(t, s, "usr_ada")

	theirs, _ := identity.NewTOTPSecret()
	err := s.Confirm(context.Background(), "usr_ada", codeAt(t, theirs, storeNow))
	if !errors.Is(err, identity.ErrBadCode) {
		t.Fatalf("another secret's code confirmed this enrolment: %v", err)
	}
}

/*
A code is spent when it is used.

It is valid for a whole thirty-second step, which is long enough to be read off
a shoulder, a shared screen or a screenshot and typed somewhere else. Without
this, a second factor is a password that changes every half minute — better than
nothing, and not what it claims to be.
*/
func TestACodeCannotBeUsedTwice(t *testing.T) {
	s := open(t)
	secret := enrolled(t, s, "usr_ada")
	if err := s.Confirm(context.Background(), "usr_ada", codeAt(t, secret, storeNow)); err != nil {
		t.Fatal(err)
	}

	// The next step, so it is not the one enrolment already spent.
	next := storeNow.Add(identity.Step)
	code := codeAt(t, secret, next)

	// The store's clock does not move, so this is checked from a moment the
	// drift window reaches.
	if err := s.CheckFactor(context.Background(), "usr_ada", code); err != nil {
		t.Fatalf("a fresh code was refused: %v", err)
	}
	if err := s.CheckFactor(context.Background(), "usr_ada", code); !errors.Is(err, identity.ErrCodeUsed) {
		t.Fatalf("the same code worked twice: %v", err)
	}
}

/*
And two sign-ins racing with one stolen code let one in, not two.

Checked with a read and then written, both see a step newer than the last and
both proceed. The condition has to be inside the write.
*/
func TestTwoSignInsWithOneCodeLetOneIn(t *testing.T) {
	s := open(t)
	secret := enrolled(t, s, "usr_ada")
	if err := s.Confirm(context.Background(), "usr_ada", codeAt(t, secret, storeNow)); err != nil {
		t.Fatal(err)
	}
	code := codeAt(t, secret, storeNow.Add(identity.Step))

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		letIn int
	)
	start := make(chan struct{})
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := s.CheckFactor(context.Background(), "usr_ada", code); err == nil {
				mu.Lock()
				letIn++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if letIn != 1 {
		t.Fatalf("one code let %d sign-ins through", letIn)
	}
}

// An account with no factor is not one a code opens. The sign-in path asks
// this before it asks anything else.
func TestAnAccountWithoutAFactorRefusesEveryCode(t *testing.T) {
	s := open(t)
	person(t, s, "usr_ada")

	if err := s.CheckFactor(context.Background(), "usr_ada", "123456"); !errors.Is(err, identity.ErrNoFactor) {
		t.Fatalf("got %v", err)
	}
}

/*
The secret stops being readable the moment enrolment is confirmed.

An endpoint that hands it back turns a stolen session into a permanent second
factor of the attacker's own — one they can keep using after the password is
changed and the sessions are cut.
*/
func TestAConfirmedSecretCannotBeReadBack(t *testing.T) {
	s := open(t)
	secret := enrolled(t, s, "usr_ada")

	// Readable while the enrolment is in progress, which is how the QR code is
	// shown again to somebody who reloaded the page.
	if got, err := s.Enrolling(context.Background(), "usr_ada"); err != nil || got != secret {
		t.Fatalf("mid-enrolment: %q %v", got, err)
	}

	if err := s.Confirm(context.Background(), "usr_ada", codeAt(t, secret, storeNow)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enrolling(context.Background(), "usr_ada"); !errors.Is(err, identity.ErrFactorExists) {
		t.Fatalf("a confirmed secret was handed back: %v", err)
	}

	// Nor through the account page.
	f, err := s.FactorOf(context.Background(), "usr_ada")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.Label, secret) {
		t.Fatal("the secret is in what the account page reads")
	}
}

// Starting another enrolment does not silently replace a confirmed factor.
// Turning one off is its own act; doing it as a side effect would be a way to
// downgrade an account by asking politely.
func TestAConfirmedFactorIsNotReplacedByStartingAnother(t *testing.T) {
	s := open(t)
	secret := enrolled(t, s, "usr_ada")
	if err := s.Confirm(context.Background(), "usr_ada", codeAt(t, secret, storeNow)); err != nil {
		t.Fatal(err)
	}

	another, _ := identity.NewTOTPSecret()
	if err := s.Enrol(context.Background(), "usr_ada", another, "A new phone"); !errors.Is(err, identity.ErrFactorExists) {
		t.Fatalf("the confirmed factor was replaced: %v", err)
	}
	// And the original still works.
	if err := s.CheckFactor(context.Background(), "usr_ada",
		codeAt(t, secret, storeNow.Add(identity.Step))); err != nil {
		t.Fatalf("the original factor stopped working: %v", err)
	}
}

// An abandoned enrolment can be started again. Somebody who lost the QR code
// halfway is the ordinary case, and a half-enrolment they cannot clear is a
// support ticket.
func TestAnAbandonedEnrolmentCanBeStartedAgain(t *testing.T) {
	s := open(t)
	enrolled(t, s, "usr_ada")

	another, _ := identity.NewTOTPSecret()
	if err := s.Enrol(context.Background(), "usr_ada", another, "Second try"); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm(context.Background(), "usr_ada", codeAt(t, another, storeNow)); err != nil {
		t.Fatalf("the second attempt could not be confirmed: %v", err)
	}
}

func TestARecoveryCodeIsGoodOnce(t *testing.T) {
	s := open(t)
	person(t, s, "usr_ada")

	codes, hashes, err := identity.NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRecoveryCodes(context.Background(), "usr_ada", hashes); err != nil {
		t.Fatal(err)
	}

	if err := s.SpendRecoveryCode(context.Background(), "usr_ada", codes[0]); err != nil {
		t.Fatalf("a fresh recovery code was refused: %v", err)
	}
	if err := s.SpendRecoveryCode(context.Background(), "usr_ada", codes[0]); !errors.Is(err, identity.ErrBadCode) {
		t.Fatalf("a spent recovery code worked again: %v", err)
	}
	// The others are untouched.
	if err := s.SpendRecoveryCode(context.Background(), "usr_ada", codes[1]); err != nil {
		t.Fatalf("spending one invalidated the rest: %v", err)
	}
}

// Regenerating replaces the set, which is the point: a sheet that has been
// photographed is replaced by asking for new ones.
func TestRegeneratingRetiresTheOldCodes(t *testing.T) {
	s := open(t)
	person(t, s, "usr_ada")

	old, oldHashes, _ := identity.NewRecoveryCodes()
	if err := s.SetRecoveryCodes(context.Background(), "usr_ada", oldHashes); err != nil {
		t.Fatal(err)
	}
	fresh, freshHashes, _ := identity.NewRecoveryCodes()
	if err := s.SetRecoveryCodes(context.Background(), "usr_ada", freshHashes); err != nil {
		t.Fatal(err)
	}

	if err := s.SpendRecoveryCode(context.Background(), "usr_ada", old[0]); !errors.Is(err, identity.ErrBadCode) {
		t.Fatal("an old recovery code still works")
	}
	if err := s.SpendRecoveryCode(context.Background(), "usr_ada", fresh[0]); err != nil {
		t.Fatalf("a new one does not: %v", err)
	}
}

/*
Removing the factor takes the recovery codes with it.

Codes left behind are ten live credentials for an account with no second factor
— not a lesser risk than the factor was, but a set of passwords the owner
believes are inert.
*/
func TestRemovingTheFactorRemovesTheCodes(t *testing.T) {
	s := open(t)
	secret := enrolled(t, s, "usr_ada")
	if err := s.Confirm(context.Background(), "usr_ada", codeAt(t, secret, storeNow)); err != nil {
		t.Fatal(err)
	}
	codes, hashes, _ := identity.NewRecoveryCodes()
	if err := s.SetRecoveryCodes(context.Background(), "usr_ada", hashes); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveFactor(context.Background(), "usr_ada"); err != nil {
		t.Fatal(err)
	}
	if s.Protected(context.Background(), "usr_ada") {
		t.Fatal("the account is still protected")
	}
	if err := s.SpendRecoveryCode(context.Background(), "usr_ada", codes[0]); !errors.Is(err, identity.ErrBadCode) {
		t.Fatal("a recovery code outlived the factor it belonged to")
	}
}

// One account's factor is not another's, which a shared row or a query missing
// its user_id would break in the worst possible way.
func TestOneAccountsFactorIsItsOwn(t *testing.T) {
	s := open(t)
	mine := enrolled(t, s, "usr_ada")
	if err := s.Confirm(context.Background(), "usr_ada", codeAt(t, mine, storeNow)); err != nil {
		t.Fatal(err)
	}
	person(t, s, "usr_grace")

	if s.Protected(context.Background(), "usr_grace") {
		t.Fatal("grace is protected by ada's factor")
	}
	if err := s.CheckFactor(context.Background(), "usr_grace",
		codeAt(t, mine, storeNow.Add(identity.Step))); !errors.Is(err, identity.ErrNoFactor) {
		t.Fatalf("ada's code was checked against grace's account: %v", err)
	}
}
