package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/extension"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Signing out of cronos is not signing out.

Until this, ending a session cleared the browser's storage and stopped there.
The identity provider's session was untouched, so clicking sign out and clicking
sign in again put the same person straight back in with nothing asked — which
looks like the button did not work, and on a shared machine means the next
person is signed in as the last one.

These are the tests for the other half: where to send the browser, and what
must not be in it.
*/

// endSession is a provider that publishes one, and records what it was asked.
type endSession struct {
	fakeFlow
	gotHint  string
	endpoint string
}

func (e *endSession) SignOut(hint string) string {
	e.gotHint = hint
	if e.endpoint == "" {
		return ""
	}
	out := e.endpoint + "?client_id=portal"
	if hint != "" {
		out += "&id_token_hint=" + hint
	}
	return out
}

// fakeFlow is a sign-in that succeeds and hands back an identity token.
type fakeFlow struct {
	identity extension.Identity
}

func (f *fakeFlow) Name() string { return "fake" }

func (f *fakeFlow) Start(_ context.Context, returning string) (string, extension.State, error) {
	return "https://idp.example/authorize", extension.State{
		ID: "state-1", Data: map[string]string{"returning": returning},
		Expires: time.Now().Add(time.Minute),
	}, nil
}

func (f *fakeFlow) Complete(_ context.Context, _ *http.Request,
	s extension.State) (extension.Identity, error) {

	who := f.identity
	who.Returning = s.Data["returning"]
	return who, nil
}

// signedIn is a directory that accepts whoever arrives.
type signedIn struct{}

func (signedIn) Upsert(_ context.Context, u identity.User) (identity.User, error) { return u, nil }

// arrive walks a whole sign-in through the handler and returns the session.
func arrive(t *testing.T, h *api.SSO) string {
	t.Helper()

	start := httptest.NewRecorder()
	h.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/v1/auth/sso/start", nil))

	back := httptest.NewRequest(http.MethodGet, "/v1/auth/sso/callback?code=x", nil)
	for _, c := range start.Result().Cookies() {
		back.AddCookie(c)
	}
	done := httptest.NewRecorder()
	h.ServeHTTP(done, back)

	where := done.Result().Header.Get("Location")
	_, fragment, ok := strings.Cut(where, "#token=")
	if !ok {
		t.Fatalf("sign-in did not hand back a session: %s (%d)", where, done.Code)
	}
	return fragment
}

func handler(t *testing.T, flow extension.SignInFlow) (*api.SSO, *token.Signer) {
	t.Helper()

	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return api.NewSSO(flow, signer, signedIn{}, api.NewAuthor(signer, nil), quiet()).
		In("acme", "finance", "viewer"), signer
}

func logout(t *testing.T, h *api.SSO, session string) map[string]string {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/v1/auth/sso/logout", nil)
	if session != "" {
		r.Header.Set("Authorization", "Bearer "+session)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("sign-out answered %d: %s", w.Code, w.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("sign-out answered %q", w.Body.String())
	}
	return out
}

func TestSigningOutEndsTheProvidersSessionToo(t *testing.T) {
	flow := &endSession{
		fakeFlow: fakeFlow{identity: extension.Identity{
			Subject: "u1", Email: "ada@example.com", Token: "the-id-token",
		}},
		endpoint: "https://idp.example/logout",
	}
	h, _ := handler(t, flow)

	session := arrive(t, h)
	out := logout(t, h, session)

	if !strings.HasPrefix(out["redirect"], "https://idp.example/logout") {
		t.Fatalf("nowhere to send the browser: %q", out["redirect"])
	}
	// The hint is what lets the provider know whose session to end. Okta
	// refuses without one.
	if flow.gotHint != "the-id-token" {
		t.Fatalf("the identity token was not carried through: %q", flow.gotHint)
	}
}

/*
The identity token is never in the answer, and never in the session.

It is a credential at the provider. In the JSON it is a credential in a page and
in the browser's network log; inside the cronos session it is one in
localStorage, readable by any script the portal ever loads. The only copy is in
this process's memory.
*/
func TestTheIdentityTokenNeverReachesTheBrowser(t *testing.T) {
	const secret = "the-id-token"

	flow := &endSession{
		fakeFlow: fakeFlow{identity: extension.Identity{Subject: "u1", Token: secret}},
	}
	h, _ := handler(t, flow)

	// It must not come back from the sign-in that produced it...
	session := arrive(t, h)
	if strings.Contains(session, secret) {
		t.Fatal("the identity token is inside the cronos session")
	}

	// ...nor from the sign-out that spends it. This provider publishes no
	// end-session endpoint, so there is nowhere to go and nothing to say.
	out := logout(t, h, session)
	if out["redirect"] != "" {
		t.Fatalf("a provider with no end-session endpoint sent us to %q", out["redirect"])
	}
}

/*
A provider that cannot end its own session says so plainly.

Plenty publish no end_session_endpoint. Guessing one — /logout, /v2/logout,
whatever the last provider used — sends somebody to a 404 on a domain they
recognise, which reads as cronos being broken and their session still being
open. It is, but only the second half is worth saying.
*/
func TestAProviderWithoutSignOutIsNotGuessedAt(t *testing.T) {
	h, _ := handler(t, &fakeFlow{identity: extension.Identity{Subject: "u1", Token: "t"}})

	out := logout(t, h, arrive(t, h))
	if out["redirect"] != "" {
		t.Fatalf("a provider with no sign-out was sent to %q", out["redirect"])
	}
}

/*
The hint is spent once.

Two sign-outs of the same session is one person clicking twice and one browser
replaying a request. The second gets the endpoint without a hint rather than a
second copy of a credential, and a provider that refuses that is refusing a
sign-out for a session that already ended.
*/
func TestAHintIsGoodForOneSignOut(t *testing.T) {
	flow := &endSession{
		fakeFlow: fakeFlow{identity: extension.Identity{Subject: "u1", Token: "the-id-token"}},
		endpoint: "https://idp.example/logout",
	}
	h, _ := handler(t, flow)

	session := arrive(t, h)
	logout(t, h, session)
	if flow.gotHint != "the-id-token" {
		t.Fatalf("the first sign-out went without a hint: %q", flow.gotHint)
	}

	logout(t, h, session)
	if flow.gotHint != "" {
		t.Fatalf("the hint survived being spent: %q", flow.gotHint)
	}
}

/*
Somebody else's hint is not yours.

The route reads the session it is ending and looks up that account's token. If
it read anything from the request instead — a query parameter, a body, a header
— then signing out would be a way to be handed another person's credential at
the identity provider by asking for it.
*/
func TestASignOutCannotSpendSomebodyElsesHint(t *testing.T) {
	flow := &endSession{
		fakeFlow: fakeFlow{identity: extension.Identity{Subject: "ada", Token: "adas-token"}},
		endpoint: "https://idp.example/logout",
	}
	h, signer := handler(t, flow)

	arrive(t, h) // Ada signs in; her token is held.

	// Grace has a perfectly good session of her own and asks to sign out.
	grace, err := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "viewer",
		Org: "acme", Project: "finance", Subject: "sso_fake_grace",
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// Asking for Ada by every name a request can carry one under. The point is
	// not that these parameters exist — they do not — but that adding one later
	// fails here rather than in somebody's log.
	for _, named := range []string{
		"?user=sso_fake_ada", "?subject=sso_fake_ada", "?sub=sso_fake_ada",
		"?email=ada@example.com", "?hint=adas-token", "?id_token_hint=adas-token",
	} {
		flow.gotHint = ""

		r := httptest.NewRequest(http.MethodPost, "/v1/auth/sso/logout"+named, nil)
		r.Header.Set("Authorization", "Bearer "+grace)
		r.Header.Set("X-User", "sso_fake_ada")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)

		if flow.gotHint != "" {
			t.Fatalf("grace's sign-out with %s carried %q", named, flow.gotHint)
		}
		if strings.Contains(w.Body.String(), "adas-token") {
			t.Fatalf("grace was handed ada's credential with %s: %s", named, w.Body)
		}
	}

	// And Ada's is still hers, unspent by any of that.
	if hint := flow.gotHint; hint != "" {
		t.Fatalf("something was spent: %q", hint)
	}
	logout(t, h, arrive(t, h))
	if flow.gotHint != "adas-token" {
		t.Fatalf("ada's own sign-out went without her hint: %q", flow.gotHint)
	}
}

/*
An expired session leaves nothing behind.

Somebody who signs in and closes the tab never calls this route, so nothing
would ever remove their hint. Over a month a long-lived process would hold one
provider credential per person who has ever signed in — a heap dump nobody
thought was sensitive.
*/
func TestAHintDoesNotOutliveItsSession(t *testing.T) {
	flow := &endSession{
		fakeFlow: fakeFlow{identity: extension.Identity{Subject: "u1", Token: "the-id-token"}},
		endpoint: "https://idp.example/logout",
	}
	h, _ := handler(t, flow)

	session := arrive(t, h)

	// A day later, after the session it belonged to has expired.
	api.SSOClock(h, func() time.Time { return time.Now().Add(25 * time.Hour) })

	logout(t, h, session)
	if flow.gotHint != "" {
		t.Fatalf("a hint outlived its session: %q", flow.gotHint)
	}
}

/*
Nothing in the request decides where the browser lands.

The provider redirects there, so a value taken from the URL would be an open
redirect on the one route somebody reaches expecting to have just left — and the
page they land on can say "your session expired, sign in again" convincingly.
The landing is deployment configuration and the provider only accepts one it
already knows, which is a stronger answer than checking the string.
*/
func TestNothingInASignOutRequestDecidesWhereItLands(t *testing.T) {
	flow := &endSession{
		fakeFlow: fakeFlow{identity: extension.Identity{Subject: "u1", Token: "t"}},
		endpoint: "https://idp.example/logout",
	}
	h, _ := handler(t, flow)
	session := arrive(t, h)

	for _, named := range []string{
		"?returning=//evil.example/looks-like-cronos",
		"?post_logout_redirect_uri=https://evil.example",
		"?redirect=https://evil.example", "?next=https://evil.example",
	} {
		r := httptest.NewRequest(http.MethodPost, "/v1/auth/sso/logout"+named, nil)
		r.Header.Set("Authorization", "Bearer "+session)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)

		if strings.Contains(w.Body.String(), "evil.example") {
			t.Fatalf("%s reached the redirect: %s", named, w.Body)
		}
	}
}

// A GET is not a sign-out. A link, an image tag or a prefetch would otherwise
// spend somebody's hint and end their provider session from another site.
func TestSignOutRefusesAGet(t *testing.T) {
	h, _ := handler(t, &fakeFlow{})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/auth/sso/logout", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("a GET signed somebody out: %d", w.Code)
	}
}

// quiet is a logger that keeps the test output about the test.
func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
