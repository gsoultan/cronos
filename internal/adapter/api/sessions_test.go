package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Ending every session, on a token that has no session.

A portal token is signed and stateless: nothing is recorded when one is minted,
so there is nothing to walk and nothing to delete. The account page offered
"sign out everywhere else" anyway, and it filtered an array in the browser — a
security control that did nothing, next to a list of devices that was invented.

The mechanism that replaces it is one timestamp per account, and these are the
tests that it draws the line in the right place. The subtle one is the last: a
line drawn a moment too late logs out the person who drew it.
*/

// standing is an account whose state the tests move.
type standing struct {
	mu       sync.Mutex
	known    bool
	active   bool
	since    time.Time
	now      func() time.Time
	ended    []string
	noSuchID bool
}

func (s *standing) Active(_ context.Context, _ string) (bool, bool, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.known, s.active, s.since
}

func (s *standing) EndSessions(_ context.Context, id string) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.noSuchID {
		return time.Time{}, identity.ErrNoUser
	}
	s.ended = append(s.ended, id)
	// The next second, like the real store: everything minted up to and
	// including this one is on the far side of the line.
	s.since = s.now().Truncate(time.Second).Add(time.Second)
	return s.since, nil
}

// The rest of Roster, unused here but required by the interface.
func (s *standing) People(context.Context, principal.Principal) ([]identity.User, error) {
	return nil, nil
}
func (s *standing) CreateUser(context.Context, identity.User, string) error { return nil }
func (s *standing) SetRole(context.Context, principal.Principal, string, string) error {
	return nil
}
func (s *standing) SetDisabled(context.Context, principal.Principal, string, bool) error {
	return nil
}
func (s *standing) ChangePassword(context.Context, string, string, string) error { return nil }

func (s *standing) Me(_ context.Context, id string) (identity.User, error) {
	return identity.User{ID: id, Email: "ada@acme.example", Name: "Ada"}, nil
}
func (s *standing) SetName(context.Context, string, string) error { return nil }

func account(t *testing.T) (*standing, *api.Author, *token.Signer) {
	t.Helper()

	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	who := &standing{known: true, active: true, now: time.Now}
	return who, api.NewAuthor(signer, nil).WithStanding(who), signer
}

// session mints one, as of a moment.
func session(t *testing.T, signer *token.Signer, at time.Time) string {
	t.Helper()

	// Dated from `at` rather than from now, which is what WithClock is for:
	// this is the same seam a scheduled mint uses to date a token from the run
	// rather than from whenever a worker got to it.
	issued, err := signer.WithClock(func() time.Time { return at }).Mint(token.Claims{
		Audience: token.Portal, Role: "editor",
		Org: "acme", Project: "finance", Subject: "usr_ada",
	}, api.SessionLifetime)
	if err != nil {
		t.Fatal(err)
	}
	return issued
}

// accepted asks the authoriser whether this token still acts.
func accepted(author *api.Author, session string) bool {
	r := httptest.NewRequest(http.MethodGet, "/v1/catalog", nil)
	r.Header.Set("Authorization", "Bearer "+session)
	_, ok := author.Principal(r)
	return ok
}

func TestATokenMintedBeforeTheLineStopsWorking(t *testing.T) {
	who, author, signer := account(t)

	// A laptop signed in this morning.
	stolen := session(t, signer, time.Now().Add(-4*time.Hour))
	if !accepted(author, stolen) {
		t.Fatal("a good token was refused before anything happened")
	}

	// The line is drawn now.
	who.mu.Lock()
	who.since = time.Now()
	who.mu.Unlock()

	// The cached answer is a few seconds old, which is the same trade already
	// made for disabling somebody. Past it, the token is dead.
	api.ForgetStanding(author)

	if accepted(author, stolen) {
		t.Fatal("a token minted before the line still works")
	}
}

/*
The line does not log out whoever drew it.

`iat` has second granularity, so a token minted in the same second as the
cut-off must survive it. With `>` rather than `>=`, the new token this endpoint
hands back is refused by the very line it just drew — and somebody reacting to a
stolen laptop is bounced to a password prompt for doing the right thing.
*/
func TestTheLineSparesATokenMintedInTheSameSecond(t *testing.T) {
	who, author, signer := account(t)

	drawn := time.Now()
	who.mu.Lock()
	who.since = drawn
	who.mu.Unlock()
	api.ForgetStanding(author)

	// Minted in the same second, which is what happens when the endpoint draws
	// the line and mints a replacement in one request.
	fresh := session(t, signer, drawn)
	if !accepted(author, fresh) {
		t.Fatal("the token minted alongside the line was refused by it")
	}
}

// A machine credential is not an account, so no line applies to it. Refusing
// them was the bug this check had the first time: every `cronos-token`
// credential a deployment mints would stop working.
func TestAMachineCredentialIsUntouched(t *testing.T) {
	who, author, signer := account(t)

	who.mu.Lock()
	who.known, who.since = false, time.Now()
	who.mu.Unlock()

	old := session(t, signer, time.Now().Add(-time.Hour))
	if !accepted(author, old) {
		t.Fatal("a machine credential was ended by somebody else's sign-out")
	}
}

// No line drawn is every account until somebody presses the button, and it must
// not refuse anything.
func TestNoLineRefusesNothing(t *testing.T) {
	_, author, signer := account(t)

	for _, age := range []time.Duration{0, time.Hour, 7 * time.Hour} {
		if !accepted(author, session(t, signer, time.Now().Add(-age))) {
			t.Fatalf("a %s-old token was refused with no line drawn", age)
		}
	}
}

/*
Pressing it ends the others and keeps this one.

The whole product claim. Without the fresh token the endpoint is "sign out
everywhere", including here — which is defensible but is not what the button
says, and is a worse thing to do to somebody who has just lost a laptop.
*/
func TestEndingSessionsKeepsTheBrowserThatDidIt(t *testing.T) {
	who, author, signer := account(t)

	elsewhere := session(t, signer, time.Now().Add(-time.Hour))
	here := session(t, signer, time.Now().Add(-time.Hour))

	h := api.NewSessions(who, author, signer, quiet())

	r := httptest.NewRequest(http.MethodPost, "/v1/auth/sessions/end", nil)
	r.Header.Set("Authorization", "Bearer "+here)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("ending sessions answered %d: %s", w.Code, w.Body)
	}
	var out struct{ Token string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Token == "" {
		t.Fatal("no replacement session came back, so this browser was signed out too")
	}

	api.ForgetStanding(author)

	if accepted(author, elsewhere) {
		t.Fatal("the other device still works")
	}
	if accepted(author, here) {
		t.Fatal("the old token here still works")
	}
	if !accepted(author, out.Token) {
		t.Fatal("the replacement does not work, so this is sign-out-everywhere")
	}
}

// It ends the caller's own sessions and nobody else's. The subject comes from
// the token; a body or a query that could name somebody would make this a way
// to sign anybody out.
func TestEndingSessionsCannotNameSomebodyElse(t *testing.T) {
	who, author, signer := account(t)
	h := api.NewSessions(who, author, signer, quiet())

	r := httptest.NewRequest(http.MethodPost,
		"/v1/auth/sessions/end?user=usr_grace&id=usr_grace", nil)
	r.Header.Set("Authorization", "Bearer "+session(t, signer, time.Now()))
	h.ServeHTTP(httptest.NewRecorder(), r)

	who.mu.Lock()
	defer who.mu.Unlock()
	if len(who.ended) != 1 || who.ended[0] != "usr_ada" {
		t.Fatalf("ended %v", who.ended)
	}
}

// A GET is not a sign-out. A link or a prefetch would otherwise end every
// session somebody has from another site.
func TestEndingSessionsRefusesAGet(t *testing.T) {
	who, author, signer := account(t)
	h := api.NewSessions(who, author, signer, quiet())

	r := httptest.NewRequest(http.MethodGet, "/v1/auth/sessions/end", nil)
	r.Header.Set("Authorization", "Bearer "+session(t, signer, time.Now()))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("a GET ended somebody's sessions: %d", w.Code)
	}
}

// And it needs a session of its own. Without one there is nobody to end.
func TestEndingSessionsNeedsASession(t *testing.T) {
	who, author, signer := account(t)
	h := api.NewSessions(who, author, signer, quiet())

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/auth/sessions/end", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("answered %d", w.Code)
	}
}

/*
A session minted in the same second as the press is still ended.

Found by driving it rather than by reasoning about it: two browsers, a phone
signed in 900ms before the button, and the phone survived. `iat` has second
granularity, so the line and the token it should kill landed in the same second
and the comparison — which has to spare same-second tokens, or the replacement
is refused by the line it just drew — spared it too.

The line is rounded up to the next second, which removes the ambiguity instead
of narrowing it. This is the test that it stays removed, and it is the one that
a version comparing against an unrounded `now` fails.
*/
func TestASessionMintedInThePressedSecondIsEnded(t *testing.T) {
	who, author, signer := account(t)

	// Both inside one second, the way two sign-ins moments apart are.
	moment := time.Now()
	who.now = func() time.Time { return moment }

	phone := session(t, signer, moment)
	here := session(t, signer, moment)

	h := api.NewSessions(who, author, signer, quiet())
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/sessions/end", nil)
	r.Header.Set("Authorization", "Bearer "+here)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	var out struct{ Token string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%d: %s", w.Code, w.Body)
	}
	api.ForgetStanding(author)

	if accepted(author, phone) {
		t.Fatal("a session minted in the same second as the press survived it")
	}
	if !accepted(author, out.Token) {
		t.Fatal("the replacement was refused by the line it drew")
	}
}
