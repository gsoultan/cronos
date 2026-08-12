package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Signing in when there is a second factor.

The mechanism is the store's. What this decides is the shape of the exchange,
and the shape is where a correct second factor is still made worse than useless:

  - if the *first* answer differs for a protected account, anybody can find out
    which accounts have one, and which passwords are therefore worth buying;
  - if the code is checked before the password, the same thing, more loudly;
  - if a wrong password and a wrong code are told apart, an attacker who has one
    of the two knows which they still need.
*/

// signers authenticates two people, which is what makes "whose factor" a real
// question rather than one a single-account fake answers by accident.
type signers struct{ people []identity.User }

func (s signers) Authenticate(_ context.Context, email, password string) (identity.User, error) {
	if password != "a-password-they-chose" {
		return identity.User{}, identity.ErrBadCredentials
	}
	for _, person := range s.people {
		if person.Email == email {
			return person, nil
		}
	}
	return identity.User{}, identity.ErrBadCredentials
}

func signInWith(t *testing.T, h http.Handler, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	encoded, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(string(encoded)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func protectedSignIn(t *testing.T) (*guarded, http.Handler) {
	t.Helper()

	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	rows := newGuarded()
	users := signers{people: []identity.User{
		{ID: "usr_ada", Email: "ada@acme.example",
			Org: "acme", Project: "finance", Role: "editor"},
		// Grace has no second factor. She exists so that "is this account
		// protected" is asked about the person signing in rather than about
		// whoever the code happens to reach.
		{ID: "usr_grace", Email: "grace@acme.example",
			Org: "acme", Project: "finance", Role: "editor"},
	}}
	return rows, api.NewAuth(users, signer, quiet()).WithFactors(rows)
}

// enrol turns protection on directly, the way the account page would have.
func enrol(t *testing.T, rows *guarded) string {
	t.Helper()

	secret, err := identity.NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.Enrol(context.Background(), "usr_ada", secret, "Authenticator app"); err != nil {
		t.Fatal(err)
	}
	code, _ := identity.TOTPCode(secret, rows.now)
	if err := rows.Confirm(context.Background(), "usr_ada", code); err != nil {
		t.Fatal(err)
	}
	return secret
}

func TestAProtectedAccountNeedsTheCode(t *testing.T) {
	rows, h := protectedSignIn(t)
	secret := enrol(t, rows)

	// The password alone is not a session.
	w := signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-password-they-chose",
	})
	if strings.Contains(w.Body.String(), `"token"`) {
		t.Fatalf("the password alone signed them in: %s", w.Body)
	}
	if !strings.Contains(w.Body.String(), `"factorRequired":true`) {
		t.Fatalf("the portal is not told to ask for a code: %s", w.Body)
	}

	// The password and a current code is.
	code, _ := identity.TOTPCode(secret, rows.now.Add(identity.Step))
	w = signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-password-they-chose", "code": code,
	})
	if !strings.Contains(w.Body.String(), `"token"`) {
		t.Fatalf("a right password and a right code did not sign in: %d %s", w.Code, w.Body)
	}
}

/*
Which accounts have a second factor is not something a stranger can find out.

Both the wrong-password answer and the no-such-account answer must be identical
whether or not the account is protected. If they differ, this endpoint becomes a
way to build a list of the accounts worth attacking — the ones whose owners
cared enough to enrol.
*/
func TestSignInDoesNotSayWhoIsProtected(t *testing.T) {
	unprotected, plain := protectedSignIn(t)
	_ = unprotected
	rows, guardedH := protectedSignIn(t)
	enrol(t, rows)

	for _, attempt := range []map[string]string{
		{"email": "ada@acme.example", "password": "a-wrong-password"},
		{"email": "nobody@acme.example", "password": "a-wrong-password"},
	} {
		a := signInWith(t, plain, attempt)
		b := signInWith(t, guardedH, attempt)

		if a.Code != b.Code || a.Body.String() != b.Body.String() {
			t.Fatalf("a protected account answers differently for %v:\n  plain:     %d %s\n  protected: %d %s",
				attempt, a.Code, a.Body, b.Code, b.Body)
		}
	}
}

// The code is checked after the password, never instead of it. A wrong password
// with a right code is not a sign-in and does not say the code was right.
func TestAWrongPasswordWithARightCodeIsNothing(t *testing.T) {
	rows, h := protectedSignIn(t)
	secret := enrol(t, rows)
	code, _ := identity.TOTPCode(secret, rows.now.Add(identity.Step))

	w := signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-wrong-password", "code": code,
	})
	if strings.Contains(w.Body.String(), `"token"`) {
		t.Fatal("a wrong password signed in with a right code")
	}
	// And the answer is the plain wrong-password one — it must not mention the
	// code at all, which would confirm the code was right.
	if strings.Contains(strings.ToLower(w.Body.String()), "code") {
		t.Fatalf("the answer talks about the code: %s", w.Body)
	}
}

// A recovery code signs in too, and is spent. Somebody who lost the phone has
// only this, and sending them to an administrator instead is how a second
// factor gets removed over chat.
func TestARecoveryCodeSignsInOnce(t *testing.T) {
	rows, h := protectedSignIn(t)
	enrol(t, rows)

	codes, hashes, _ := identity.NewRecoveryCodes()
	if err := rows.SetRecoveryCodes(context.Background(), "usr_ada", hashes); err != nil {
		t.Fatal(err)
	}

	w := signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-password-they-chose", "code": codes[0],
	})
	if !strings.Contains(w.Body.String(), `"token"`) {
		t.Fatalf("a recovery code did not sign in: %d %s", w.Code, w.Body)
	}

	// And not twice.
	w = signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-password-they-chose", "code": codes[0],
	})
	if strings.Contains(w.Body.String(), `"token"`) {
		t.Fatal("a recovery code worked twice")
	}
}

// A code that was already used says so rather than "not right", because the
// person typing it is looking at an app that still shows it and would otherwise
// try the same digits again.
func TestAUsedCodeSaysSo(t *testing.T) {
	rows, h := protectedSignIn(t)
	secret := enrol(t, rows)
	code, _ := identity.TOTPCode(secret, rows.now.Add(identity.Step))

	signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-password-they-chose", "code": code,
	})
	w := signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-password-they-chose", "code": code,
	})

	if !strings.Contains(w.Body.String(), "already been used") {
		t.Fatalf("a replayed code answered %q", w.Body)
	}
}

// An account with no second factor signs in with the password, unchanged. Most
// accounts, and the path that must not have grown a step.
func TestAnUnprotectedAccountSignsInWithThePasswordAlone(t *testing.T) {
	_, h := protectedSignIn(t)

	w := signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-password-they-chose",
	})
	if !strings.Contains(w.Body.String(), `"token"`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

/*
Somebody else's second factor does not stand between you and your account.

The check has to be about the person signing in. Asked about the wrong id it
either challenges people who have no factor — who then cannot get in at all,
because they have no app to read — or, worse, waves through somebody who does.
*/
func TestOnePersonsFactorDoesNotChallengeAnother(t *testing.T) {
	rows, h := protectedSignIn(t)
	enrol(t, rows) // ada is protected; grace is not.

	w := signInWith(t, h, map[string]string{
		"email": "grace@acme.example", "password": "a-password-they-chose",
	})
	if strings.Contains(w.Body.String(), `"factorRequired"`) {
		t.Fatalf("grace was asked for ada's second factor: %s", w.Body)
	}
	if !strings.Contains(w.Body.String(), `"token"`) {
		t.Fatalf("grace could not sign in: %d %s", w.Code, w.Body)
	}

	// And the other way: ada is still challenged.
	w = signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-password-they-chose",
	})
	if !strings.Contains(w.Body.String(), `"factorRequired":true`) {
		t.Fatalf("ada was not challenged: %s", w.Body)
	}
}

/*
Mistyping the code does not lock somebody out of their password.

Found by a live run that made a dozen sign-in attempts in two seconds and got a
429 it did not expect. The two steps shared one budget, so a person retrying a
code — because it expired, or they mistyped it, or their phone's clock had
drifted — spent the budget that exists to slow somebody working through a
password list.

Both limits still bite. They just bite the thing they are for.
*/
func TestRetryingACodeDoesNotSpendThePasswordsBudget(t *testing.T) {
	rows, h := protectedSignIn(t)
	secret := enrol(t, rows)

	// A bad morning: the password, then several wrong codes.
	signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-password-they-chose",
	})
	for range 8 {
		signInWith(t, h, map[string]string{
			"email": "ada@acme.example", "password": "a-password-they-chose", "code": "000000",
		})
	}

	// The right code still gets in. Sharing one budget, this is a 429 that
	// looks to the person like their password stopped working.
	code, _ := identity.TOTPCode(secret, rows.now.Add(identity.Step))
	w := signInWith(t, h, map[string]string{
		"email": "ada@acme.example", "password": "a-password-they-chose", "code": code,
	})
	if !strings.Contains(w.Body.String(), `"token"`) {
		t.Fatalf("locked out by retrying a code: %d %s", w.Code, w.Body)
	}
}

/*
And guessing codes is still limited.

The looser budget is not no budget. An attacker who has the password would
otherwise have unlimited attempts at six digits, which a window of three turns
from hopeless into merely slow.
*/
func TestGuessingCodesIsStillThrottled(t *testing.T) {
	rows, h := protectedSignIn(t)
	enrol(t, rows)

	var throttled bool
	for range 40 {
		w := signInWith(t, h, map[string]string{
			"email": "ada@acme.example", "password": "a-password-they-chose", "code": "000000",
		})
		if w.Code == http.StatusTooManyRequests {
			throttled = true
			break
		}
	}
	if !throttled {
		t.Fatal("forty guesses at a six-digit code went unthrottled")
	}
}

/*
And the answer that asks for a code does not clear the counter.

If it did, somebody holding the password could alternate — guess a code, then
send the password alone to reset — and have unlimited attempts at six digits
while never tripping anything.
*/
func TestAskingForACodeDoesNotResetTheCodeBudget(t *testing.T) {
	rows, h := protectedSignIn(t)
	enrol(t, rows)

	var throttled bool
	for range 40 {
		// A guess, then the reset attempt.
		w := signInWith(t, h, map[string]string{
			"email": "ada@acme.example", "password": "a-password-they-chose", "code": "000000",
		})
		if w.Code == http.StatusTooManyRequests {
			throttled = true
			break
		}
		signInWith(t, h, map[string]string{
			"email": "ada@acme.example", "password": "a-password-they-chose",
		})
	}
	if !throttled {
		t.Fatal("the counter can be cleared between guesses")
	}
}
