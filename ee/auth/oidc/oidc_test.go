package oidc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/extension"
)

/*
Every test here is a way somebody signs in as you.

A sign-in flow is code where a missing check is not a bug that shows up as a
wrong answer — it shows up as the wrong person holding a session, indefinitely,
with nothing in a log that looks unusual. So the fake provider below can be
asked to lie in each of the specific ways a real attacker would, and the
assertion is always that it is refused.
*/

func TestASignInThroughAWellBehavedProviderWorks(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()

	p := provider(t, idp)
	redirect, state, err := p.Start(context.Background(), "/reports/billing")
	if err != nil {
		t.Fatal(err)
	}

	// PKCE, not just state: the challenge must be sent, and it must be the
	// S256 of a verifier this kept rather than the verifier itself.
	sent, _ := url.Parse(redirect)
	if sent.Query().Get("code_challenge") == "" ||
		sent.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("no PKCE challenge: %s", redirect)
	}
	if sent.Query().Get("code_challenge") == state.Data["verifier"] {
		t.Fatal("the verifier was sent as the challenge")
	}

	// As a real provider does: the nonce from the authorization request is
	// echoed into the token, and comparing them is what ties this answer to
	// this request.
	idp.nonce = state.Data["nonce"]

	who, err := p.Complete(context.Background(), callback(state.ID, "any-code"), state)
	if err != nil {
		t.Fatal(err)
	}
	if who.Subject != "sub-1" || who.Email != "dewi@acme.example" {
		t.Fatalf("got %+v", who)
	}
	// Carried through the round trip, so somebody who clicked a report link
	// lands on the report rather than the front page.
	if who.Returning != "/reports/billing" {
		t.Fatalf("returning is %q", who.Returning)
	}
}

/*
The classic: a token that asks to be verified with no signature at all, or with
the algorithm swapped for one whose key is public.
*/
func TestATokenThatNamesItsOwnAlgorithmIsRefused(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	p := provider(t, idp)

	for _, alg := range []string{"none", "HS256", "ES256"} {
		idp.alg = alg
		idp.nonce = "" // re-echoed for each attempt, as a real provider would
		_, err := signIn(t, p, idp)

		if err == nil {
			t.Fatalf("a token signed with %q was accepted", alg)
		}
		if !strings.Contains(err.Error(), "refusing a token signed with") {
			t.Errorf("%q was refused for the wrong reason: %v", alg, err)
		}
	}
}

// A token signed by somebody else's key, presented as the provider's.
func TestATokenSignedByAnotherKeyIsRefused(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	p := provider(t, idp)

	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp.signWith = other

	if _, err := signIn(t, p, idp); err == nil {
		t.Fatal("a token signed with a key the provider does not publish was accepted")
	}
}

/*
A token minted for a different application at the same provider.

Every tenant of a shared identity provider can obtain one of these for
themselves. Without the audience check, any of them is a session here.
*/
func TestATokenForAnotherApplicationIsRefused(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	p := provider(t, idp)

	idp.audience = "some-other-application"

	_, err := signIn(t, p, idp)
	if err == nil || !strings.Contains(err.Error(), "not minted for this application") {
		t.Fatalf("got %v", err)
	}
}

// Replaying a token captured from a different sign-in.
func TestATokenWithTheWrongNonceIsRefused(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	p := provider(t, idp)

	idp.nonce = "a nonce from somebody else's sign-in"
	_, state, _ := p.Start(context.Background(), "/")

	_, err := p.Complete(context.Background(), callback(state.ID, "code"), state)
	if err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("got %v", err)
	}
}

func TestAnExpiredTokenIsRefused(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	p := provider(t, idp)

	idp.expiry = time.Now().Add(-2 * time.Hour)

	_, err := signIn(t, p, idp)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("got %v", err)
	}
}

// The provider's answer belongs to a sign-in this browser did not start.
func TestAMismatchedStateIsRefused(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	p := provider(t, idp)

	_, state, _ := p.Start(context.Background(), "/")
	_, err := p.Complete(context.Background(), callback("somebody-elses-state", "code"), state)
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("got %v", err)
	}
}

/*
A provider that says an address is unverified is a provider where domain
restriction means nothing: anybody can claim someone@yourcompany.example.
*/
func TestAnUnverifiedAddressIsRefused(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	p := provider(t, idp)

	no := false
	idp.emailVerified = &no

	_, err := signIn(t, p, idp)
	if err == nil || !strings.Contains(err.Error(), "not a verified address") {
		t.Fatalf("got %v", err)
	}
}

func TestADomainThisDeploymentDoesNotAdmitIsRefused(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()

	cfg := config(idp)
	cfg.AllowedDomains = []string{"acme.example"}
	p := build(t, cfg)

	idp.email = "someone@gmail.example"

	_, err := signIn(t, p, idp)
	if err == nil || !strings.Contains(err.Error(), "not a domain this deployment admits") {
		t.Fatalf("got %v", err)
	}
}

/*
Discovery has to check that the document it read describes the issuer it asked
for. Without it, anything that can answer at the well-known path names its own
endpoints, and every later verification is against keys it chose.
*/
func TestADiscoveryDocumentThatNamesAnotherIssuerIsRefused(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	idp.issuer = "https://not-the-one-you-asked-for.example"

	_, err := New(context.Background(), config(idp))
	if err == nil || !strings.Contains(err.Error(), "says its issuer is") {
		t.Fatalf("got %v", err)
	}
}

// Somebody in two groups gets the stronger, because that is what whoever put
// them in both meant — and the alternative depends on a directory's iteration
// order.
func TestTheStrongestMatchingGroupWins(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()

	cfg := config(idp)
	cfg.Roles = map[string]string{"readers": "viewer", "owners": "admin", "authors": "editor"}
	p := build(t, cfg)

	idp.groups = []string{"readers", "owners", "authors"}

	who, err := signIn(t, p, idp)
	if err != nil {
		t.Fatal(err)
	}
	if who.Role != "admin" {
		t.Fatalf("role is %q", who.Role)
	}
}

// And somebody in none gets the default, which is the weakest thing: a role
// mapping that fails should give too little access rather than too much.
func TestNoMatchingGroupIsTheDefaultRole(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()

	cfg := config(idp)
	cfg.Roles = map[string]string{"owners": "admin"}
	p := build(t, cfg)

	idp.groups = []string{"some-unrelated-group"}

	who, err := signIn(t, p, idp)
	if err != nil {
		t.Fatal(err)
	}
	if who.Role != "viewer" {
		t.Fatalf("role is %q", who.Role)
	}
}

/* ---------------------------------------------------------------------- *
 * A provider that can be asked to lie.
 * ---------------------------------------------------------------------- */

type fakeIDP struct {
	*httptest.Server
	key *rsa.PrivateKey

	// What it says, and everything that can be made wrong.
	issuer   string
	alg      string
	signWith *rsa.PrivateKey
	audience string
	nonce    string
	expiry   time.Time
	// issued lets a token be minted in the future, which is a clock behind the
	// provider's rather than a token anybody could have made.
	issued time.Time
	// deny makes the authorize endpoint refuse, the way a declined consent
	// screen does.
	deny          string
	email         string
	emailVerified *bool
	groups        []string
	// endSession is what this provider publishes for RP-initiated logout, and
	// most publish nothing.
	endSession string
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	idp := &fakeIDP{key: key, alg: "RS256", email: "dewi@acme.example"}
	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		issuer := idp.issuer
		if issuer == "" {
			issuer = idp.URL
		}
		doc := map[string]string{
			"issuer":                 issuer,
			"authorization_endpoint": idp.URL + "/authorize",
			"token_endpoint":         idp.URL + "/token",
			"jwks_uri":               idp.URL + "/keys",
		}
		if idp.endSession != "" {
			doc["end_session_endpoint"] = idp.endSession
		}
		_ = json.NewEncoder(w).Encode(doc)
	})

	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "kid": "test-key",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		// A real provider checks this; the fake one asserts it was sent,
		// because a flow that quietly stopped sending it would still pass
		// every other test here.
		if r.Form.Get("code_verifier") == "" {
			http.Error(w, `{"error":"no code_verifier"}`, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": idp.mint()})
	})

	idp.Server = httptest.NewServer(mux)
	return idp
}

// mint builds an id token, honouring whatever this fake has been told to lie
// about.
func (f *fakeIDP) mint() string {
	issuer := f.issuer
	if issuer == "" {
		issuer = f.URL
	}
	aud := f.audience
	if aud == "" {
		aud = "cronos-test-client"
	}
	expiry := f.expiry
	if expiry.IsZero() {
		expiry = time.Now().Add(time.Hour)
	}
	issuedAt := f.issued
	if issuedAt.IsZero() {
		issuedAt = time.Now()
	}

	head := map[string]string{"alg": f.alg, "kid": "test-key", "typ": "JWT"}
	body := map[string]any{
		"iss": issuer, "sub": "sub-1", "aud": aud,
		"exp": expiry.Unix(), "iat": issuedAt.Unix(),
		"nonce": f.nonce, "email": f.email, "name": "Dewi",
	}
	if f.emailVerified != nil {
		body["email_verified"] = *f.emailVerified
	}
	if len(f.groups) > 0 {
		body["groups"] = f.groups
	}

	segment := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	signing := segment(head) + "." + segment(body)

	// `none` is the whole point of one of the tests: no signature at all.
	if f.alg == "none" {
		return signing + "."
	}

	key := f.signWith
	if key == nil {
		key = f.key
	}
	hasher := crypto.SHA256.New()
	hasher.Write([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hasher.Sum(nil))
	if err != nil {
		return signing + ".broken"
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func config(idp *fakeIDP) Config {
	return Config{
		Issuer:       idp.URL,
		ClientID:     "cronos-test-client",
		ClientSecret: "a-secret",
		RedirectURL:  "https://reports.example/v1/auth/sso/callback",
		Org:          "acme", Project: "finance",
	}
}

func provider(t *testing.T, idp *fakeIDP) *Provider {
	t.Helper()
	return build(t, config(idp))
}

func build(t *testing.T, cfg Config) *Provider {
	t.Helper()
	p, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// callback is the request the browser makes on the way back.
func callback(state, code string) *http.Request {
	return httptest.NewRequest(http.MethodGet,
		"/v1/auth/sso/callback?state="+url.QueryEscape(state)+"&code="+url.QueryEscape(code), nil)
}

/*
signIn runs the round trip the way it actually happens.

The nonce is echoed, because that is what a real provider does: it reads the
one in the authorization request and puts it in the token. A fake that does not
fails the nonce check on every path — which made every other test here pass for
the wrong reason. An alg-confusion test that is really a nonce-mismatch test
would keep passing with the algorithm allow-list deleted.

A test that means to break the nonce sets it first, and this leaves it alone.
*/
func signIn(t *testing.T, p *Provider, idp *fakeIDP) (extension.Identity, error) {
	t.Helper()

	_, state, err := p.Start(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if idp.nonce == "" {
		idp.nonce = state.Data["nonce"]
	}
	return p.Complete(context.Background(), callback(state.ID, "code"), state)
}

/*
Ending the provider's session.

Signing out of cronos alone leaves the person signed in where they thought they
had left: the next sign-in is silent, and on a shared machine the next person
gets the last one's session. RP-initiated logout is the other half, and it is
one URL built from three things — so these are the tests that each of the three
is right, because a wrong one produces a page at the identity provider that says
"invalid request" and nothing that says why.
*/

func TestSignOutGoesWhereTheProviderSaid(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	idp.endSession = "https://idp.example/oauth2/logout"

	p, err := New(context.Background(), config(idp))
	if err != nil {
		t.Fatal(err)
	}

	where := p.SignOut("the-id-token")
	parsed, err := url.Parse(where)
	if err != nil {
		t.Fatalf("sign-out built %q: %v", where, err)
	}

	if parsed.Scheme+"://"+parsed.Host+parsed.Path != idp.endSession {
		t.Fatalf("sign-out goes to %q", where)
	}
	// The hint says whose session. Okta refuses a logout without one.
	if got := parsed.Query().Get("id_token_hint"); got != "the-id-token" {
		t.Fatalf("id_token_hint is %q", got)
	}
	// The client id is what Entra and Keycloak accept instead, and sending
	// both is what makes one implementation work against all three.
	if got := parsed.Query().Get("client_id"); got != "cronos-test-client" {
		t.Fatalf("client_id is %q", got)
	}
}

/*
A provider that publishes no end-session endpoint is not guessed at.

Dex publishes none. Guessing /logout — because the last provider had one —
sends somebody to a 404 on a domain they recognise, which reads as cronos being
broken rather than as this provider not offering the feature.
*/
func TestSignOutIsNotInventedForAProviderThatHasNone(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()

	p, err := New(context.Background(), config(idp))
	if err != nil {
		t.Fatal(err)
	}

	if where := p.SignOut("the-id-token"); where != "" {
		t.Fatalf("a provider with no end-session endpoint sent us to %q", where)
	}
}

/*
Where the browser lands afterwards is only sent when a deployment registered
one.

post_logout_redirect_uri must match a value configured at the provider, and an
unregistered one is refused outright — turning every sign-out into an error page
at the identity provider. Sending nothing lands on the provider's own "you have
signed out" page, which is worse-looking and works.
*/
func TestAnUnconfiguredLandingIsNotSent(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	idp.endSession = "https://idp.example/logout"

	p, err := New(context.Background(), config(idp))
	if err != nil {
		t.Fatal(err)
	}

	// Nothing configured, so nothing sent: without a registered URL the
	// provider has nowhere it will agree to send anybody.
	where, _ := url.Parse(p.SignOut("t"))
	if got := where.Query().Get("post_logout_redirect_uri"); got != "" {
		t.Fatalf("an unregistered landing was sent: %q", got)
	}

	cfg := config(idp)
	cfg.PostLogoutURL = "https://portal.example/signed-out"
	p, err = New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	where, _ = url.Parse(p.SignOut("t"))
	if got := where.Query().Get("post_logout_redirect_uri"); got != cfg.PostLogoutURL {
		t.Fatalf("the configured landing was not sent: %q", got)
	}
}

// A restart loses the hints, and a sign-out then goes without one. Some
// providers refuse that; sending an empty id_token_hint= is refused by more.
func TestSignOutWithoutAHintSendsNoEmptyOne(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	idp.endSession = "https://idp.example/logout"

	p, err := New(context.Background(), config(idp))
	if err != nil {
		t.Fatal(err)
	}

	where, _ := url.Parse(p.SignOut(""))
	if _, present := where.Query()["id_token_hint"]; present {
		t.Fatalf("an empty hint was sent: %q", where)
	}
}

// The identity token has to survive the sign-in to be presentable later. It
// used to be dropped on the floor once verified, which is what made single
// log-out impossible without storing one.
func TestTheIdentityTokenIsCarriedOutOfASignIn(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()

	p, err := New(context.Background(), config(idp))
	if err != nil {
		t.Fatal(err)
	}

	who, err := signIn(t, p, idp)
	if err != nil {
		t.Fatal(err)
	}
	if who.Token == "" {
		t.Fatal("the identity token was dropped, so no sign-out can present it")
	}
	// And it is the token, not something that looks like one.
	if strings.Count(who.Token, ".") != 2 {
		t.Fatalf("what came back is not a JWT: %q", who.Token)
	}
}

/*
Which system a failure names.

Every refusal from Complete used to reach the browser as "the identity provider
refused this sign-in", and only one of them is that. The rest are cronos
refusing a token, a deployment pointed at a different client, or two machines
disagreeing about the time — and the sentence decides which admin console
somebody opens.

It is not hypothetical. A host clock jumped during a live run here, a valid
token landed outside its window, and the first thing restarted was Keycloak.
*/
func TestARefusalNamesTheSystemToLookAt(t *testing.T) {
	for _, c := range []struct {
		what  string
		set   func(*fakeIDP)
		want  error
		avoid error
	}{
		{
			what:  "a clock that is behind the provider's",
			set:   func(i *fakeIDP) { i.expiry = time.Now().Add(-2 * time.Hour) },
			want:  extension.ErrClockSkew,
			avoid: extension.ErrProviderRefused,
		},
		{
			what:  "a clock that is ahead of it",
			set:   func(i *fakeIDP) { i.issued = time.Now().Add(2 * time.Hour) },
			want:  extension.ErrClockSkew,
			avoid: extension.ErrProviderRefused,
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			idp := newFakeIDP(t)
			defer idp.Close()
			p := provider(t, idp)
			c.set(idp)

			_, err := signIn(t, p, idp)
			if err == nil {
				t.Fatal("the sign-in was allowed")
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("refused with %v, which does not say what to go and look at", err)
			}
			if errors.Is(err, c.avoid) {
				t.Fatalf("refused with %v, which sends somebody to the wrong system", err)
			}
		})
	}
}

// And the one case that has earned the default: the provider said no. It comes
// back on the callback rather than from the server — a declined consent screen
// sends the browser home with ?error= and no code.
func TestTheProviderSayingNoIsNamedAsSuch(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	p := provider(t, idp)

	_, state, err := p.Start(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	declined := httptest.NewRequest(http.MethodGet,
		"/v1/auth/sso/callback?state="+url.QueryEscape(state.ID)+"&error=access_denied", nil)

	_, err = p.Complete(context.Background(), declined, state)
	if !errors.Is(err, extension.ErrProviderRefused) {
		t.Fatalf("a provider refusal came back as %v", err)
	}
}

/*
A person the provider vouched for and this deployment will not have.

Nothing is broken and nothing will fix itself: somebody has to be admitted. It
is the one refusal where the answer is neither "check the provider" nor "check
the clocks", and it read as the first of those.
*/
func TestAnAddressOutsideTheAllowedDomainsIsNotTheProvidersFault(t *testing.T) {
	idp := newFakeIDP(t)
	defer idp.Close()
	p := provider(t, idp)
	// The fake provider signs in dewi@acme.example, so the list has to be a
	// domain that is not hers — a list containing her own would admit her and
	// this test would pass by testing nothing.
	p.cfg.AllowedDomains = []string{"globex.example"}

	_, err := signIn(t, p, idp)
	if !errors.Is(err, extension.ErrNotAcceptable) {
		t.Fatalf("an unadmitted address came back as %v", err)
	}
	if errors.Is(err, extension.ErrProviderRefused) {
		t.Fatal("an unadmitted address blamed the provider, which signed them in perfectly well")
	}
}
