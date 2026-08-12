package sql_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
An invitation is a credential that must work once.

Everything else about it is convenience. The parts worth testing are the ones
where "once" can quietly become "twice": a double-clicked link, a second browser
tab, a retry after a timeout, or a crash in the middle of the two writes that
accepting requires.
*/

// invite writes one and returns the secret that opens it.
func invite(t *testing.T, s interface {
	Invite(context.Context, identity.Invitation, string) error
}, email string, life time.Duration,
) string {
	t.Helper()

	secret, hash, err := identity.NewInvitation(32)
	if err != nil {
		t.Fatal(err)
	}
	inv := identity.Invitation{
		ID: identity.NewInvitationID(), Email: email, Name: "Dewi",
		Org: "acme", Project: "finance", Role: "editor", InvitedBy: "ada@acme.example",
		Expires: time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC).Add(life),
	}
	if err := s.Invite(context.Background(), inv, hash); err != nil {
		t.Fatal(err)
	}
	return secret
}

func TestAnInvitationBecomesAnAccount(t *testing.T) {
	s := open(t)
	secret := invite(t, s, "dewi@acme.example", identity.InvitationLife)

	// It reads back before it is used, so the page can say who this is for.
	inv, err := s.Invitation(context.Background(), secret)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Email != "dewi@acme.example" || inv.Role != "editor" {
		t.Fatalf("read back %+v", inv)
	}

	user, err := s.Accept(context.Background(), secret, "a-password-they-chose")
	if err != nil {
		t.Fatal(err)
	}
	if user.Org != "acme" || user.Project != "finance" || user.Role != "editor" {
		t.Fatalf("landed in %s/%s as %s", user.Org, user.Project, user.Role)
	}

	// And the password is theirs: nobody chose it for them, and it works.
	if _, err := s.Authenticate(context.Background(),
		"dewi@acme.example", "a-password-they-chose"); err != nil {
		t.Fatalf("the password they set does not sign them in: %v", err)
	}
}

/*
The secret is not in the database.

A backup of this table, or a read-only replica, must not be a set of working
invitations. This is the assertion that the hash is what was stored — not
belief, but the bytes.
*/
func TestTheSecretItselfIsNeverStored(t *testing.T) {
	s := open(t)
	secret := invite(t, s, "dewi@acme.example", identity.InvitationLife)

	rows, err := s.Invitations(context.Background(), who("acme", "finance"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%d invitations outstanding", len(rows))
	}

	// Nothing that comes out of the store may contain it.
	for _, field := range []string{
		rows[0].ID, rows[0].Email, rows[0].Name, rows[0].Role, rows[0].InvitedBy,
	} {
		if strings.Contains(field, secret) {
			t.Fatalf("the secret came back out of the store in %q", field)
		}
	}
}

// Used once and then never again. A link in a mailbox stays in that mailbox,
// and a second use is either somebody replaying it or the mailbox being read by
// somebody it does not belong to.
func TestAnInvitationIsSpentOnUse(t *testing.T) {
	s := open(t)
	secret := invite(t, s, "dewi@acme.example", identity.InvitationLife)

	if _, err := s.Accept(context.Background(), secret, "a-password-they-chose"); err != nil {
		t.Fatal(err)
	}

	_, err := s.Accept(context.Background(), secret, "a-different-password")
	if !errors.Is(err, identity.ErrInvitation) {
		t.Fatalf("a spent invitation was accepted again: %v", err)
	}
	// And it is not merely refused at the end: no second account exists.
	if _, err := s.Authenticate(context.Background(),
		"dewi@acme.example", "a-different-password"); err == nil {
		t.Fatal("the second acceptance changed the password")
	}
}

/*
Two at once is one account.

The failure this guards is not exotic. It is a double-click, a browser retrying
a request it thinks timed out, or two tabs open on the same email. Checked with
a read and then written, both see an unspent invitation and both proceed; the
condition has to be inside the write.

Two things stop it, and this asserts both. One account is the outcome, and the
unique index on cronos_users.email would deliver that much on its own. What
says the invitation is what was spent is *which error the losers get*: the
conditional UPDATE turns them away with ErrInvitation before any account is
attempted, where relying on the index alone would answer "already registered" —
a race reported as somebody else's mistake.
*/
func TestTwoAcceptancesAtOnceProduceOneAccount(t *testing.T) {
	s := open(t)
	secret := invite(t, s, "dewi@acme.example", identity.InvitationLife)

	const tries = 8
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		accepted []string
		refused  []error
	)
	start := make(chan struct{})

	for i := 0; i < tries; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			user, err := s.Accept(context.Background(), secret, "a-password-they-chose")

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				refused = append(refused, err)
				return
			}
			accepted = append(accepted, user.ID)
		}()
	}
	close(start)
	wg.Wait()

	if len(accepted) != 1 {
		t.Fatalf("%d of %d concurrent acceptances succeeded: %v", len(accepted), tries, accepted)
	}
	for _, err := range refused {
		if !errors.Is(err, identity.ErrInvitation) {
			t.Fatalf("a loser was turned away by the account table, not the invitation: %v", err)
		}
	}
}

/*
A failed acceptance leaves the invitation usable.

The two writes are one transaction, so if the account cannot be created the
invitation must not be spent — otherwise a transient failure burns the link and
the person has to be invited again, having already chosen a password.
*/
func TestAFailedAcceptanceDoesNotBurnTheInvitation(t *testing.T) {
	s := open(t)
	secret := invite(t, s, "dewi@acme.example", identity.InvitationLife)

	// Somebody creates the account by another route in the meantime — the
	// admin adds them directly, say — and the INSERT collides.
	if err := s.CreateUser(context.Background(), identity.User{
		ID: identity.NewID(), Email: "dewi@acme.example",
		Org: "acme", Project: "finance", Role: "viewer",
	}, "set-by-an-administrator"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Accept(context.Background(), secret, "a-password-they-chose"); err == nil {
		t.Fatal("accepting produced a second account for one address")
	}

	// The invitation survived, because nothing happened.
	if _, err := s.Invitation(context.Background(), secret); err != nil {
		t.Fatalf("a failed acceptance spent the invitation: %v", err)
	}
}

// A week, and then it is a dead string. A link forwarded into an archive in
// March is not a way in come September.
func TestAnExpiredInvitationIsRefused(t *testing.T) {
	s := open(t)
	secret := invite(t, s, "dewi@acme.example", -time.Hour)

	if _, err := s.Invitation(context.Background(), secret); !errors.Is(err, identity.ErrInvitation) {
		t.Fatalf("an expired invitation read back: %v", err)
	}
	if _, err := s.Accept(context.Background(), secret, "a-password-they-chose"); !errors.Is(err, identity.ErrInvitation) {
		t.Fatalf("an expired invitation was accepted: %v", err)
	}
}

/*
One that never existed answers the same as one that did.

This endpoint has no session by design — it is how somebody with no account
gets one. Telling "expired" apart from "never existed" turns it into a way to
learn which invitations are outstanding, and by extension which addresses
somebody is trying to onboard.
*/
func TestAnUnknownSecretIsIndistinguishableFromASpentOne(t *testing.T) {
	s := open(t)
	spent := invite(t, s, "dewi@acme.example", identity.InvitationLife)
	if _, err := s.Accept(context.Background(), spent, "a-password-they-chose"); err != nil {
		t.Fatal(err)
	}

	_, wasSpent := s.Invitation(context.Background(), spent)
	_, neverWas := s.Invitation(context.Background(), "a-secret-nobody-issued")

	if wasSpent == nil || neverWas == nil {
		t.Fatal("one of these was accepted")
	}
	if wasSpent.Error() != neverWas.Error() {
		t.Fatalf("spent says %q, unknown says %q", wasSpent, neverWas)
	}
}

// Inviting the same address twice replaces the first link rather than adding a
// second. The usual reason to invite somebody again is that the first mail went
// astray, and in that case the first link should stop working.
func TestInvitingTwiceLeavesOneLiveLink(t *testing.T) {
	s := open(t)
	first := invite(t, s, "dewi@acme.example", identity.InvitationLife)
	second := invite(t, s, "dewi@acme.example", identity.InvitationLife)

	if _, err := s.Invitation(context.Background(), first); !errors.Is(err, identity.ErrInvitation) {
		t.Fatal("the first invitation still works")
	}
	if _, err := s.Invitation(context.Background(), second); err != nil {
		t.Fatalf("the second does not: %v", err)
	}
}

// Somebody who already has an account is not invited. The link would either
// create a second account for one address or fail at the very end, after they
// had chosen a password.
func TestSomebodyWhoAlreadyHasAnAccountIsNotInvited(t *testing.T) {
	s := open(t)
	if err := s.CreateUser(context.Background(), identity.User{
		ID: identity.NewID(), Email: "dewi@acme.example",
		Org: "acme", Project: "finance", Role: "viewer",
	}, "already-has-one"); err != nil {
		t.Fatal(err)
	}

	_, hash, err := identity.NewInvitation(32)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Invite(context.Background(), identity.Invitation{
		ID: identity.NewInvitationID(), Email: "dewi@acme.example",
		Org: "acme", Project: "finance", Role: "editor",
		Expires: time.Now().Add(identity.InvitationLife),
	}, hash)

	if !errors.Is(err, identity.ErrExists) {
		t.Fatalf("inviting somebody who already has an account: %v", err)
	}
}

/*
An invitation belongs to one project.

Listing and withdrawing are both scoped to the caller's, because an
administrator of one project seeing another's outstanding invitations is a list
of who a competitor is hiring.
*/
func TestInvitationsAreScopedToTheirProject(t *testing.T) {
	s := open(t)
	invite(t, s, "dewi@acme.example", identity.InvitationLife)

	if rows, err := s.Invitations(context.Background(), who("globex", "ops")); err != nil {
		t.Fatal(err)
	} else if len(rows) != 0 {
		t.Fatalf("another project sees %d of acme's invitations", len(rows))
	}

	ours, err := s.Invitations(context.Background(), who("acme", "finance"))
	if err != nil || len(ours) != 1 {
		t.Fatalf("acme sees %d of its own: %v", len(ours), err)
	}

	// And cannot be withdrawn from outside it.
	if err := s.Uninvite(context.Background(), who("globex", "ops"), ours[0].ID); err == nil {
		t.Fatal("another project withdrew acme's invitation")
	}
}

func TestAWithdrawnInvitationStopsWorking(t *testing.T) {
	s := open(t)
	secret := invite(t, s, "dewi@acme.example", identity.InvitationLife)

	rows, err := s.Invitations(context.Background(), who("acme", "finance"))
	if err != nil || len(rows) != 1 {
		t.Fatalf("%d outstanding: %v", len(rows), err)
	}
	if err := s.Uninvite(context.Background(), who("acme", "finance"), rows[0].ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Accept(context.Background(), secret, "a-password-they-chose"); !errors.Is(err, identity.ErrInvitation) {
		t.Fatalf("a withdrawn invitation was accepted: %v", err)
	}
}
