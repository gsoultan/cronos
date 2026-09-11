package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
The enrolment fence has to survive everything a fenced session may do.

A project that requires a second factor lets an account with none sign in and
go nowhere: the session carries Enrol, and OnlyEnrolment fences it to the few
routes somebody needs in order to enrol. Ending other sessions is on that list,
because somebody enrolling may be doing it precisely because a session was
stolen.

Ending sessions mints a replacement, and the replacement did not carry Enrol.
So the fence was one request wide: sign in with a password, call
/v1/auth/sessions/end, and the session that comes back is unfenced with no
factor ever enrolled — and from there the attacker enrols their own
authenticator and locks the owner out.

The claims are asserted rather than the routes, because the fence is a property
of the token: OnlyEnrolment reads Enrol, and anything that mints a session
without carrying it forward re-opens this from a different file.
*/
func TestEndingSessionsKeepsTheEnrolmentFence(t *testing.T) {
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}

	// The session a project requiring a factor hands somebody who has none.
	fenced, err := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "admin", Org: "acme", Project: "finance",
		Subject: "usr_1", Enrol: true,
	}, token.MaxLifetime)
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/v1/auth/sessions/end", nil)
	r.Header.Set("Authorization", "Bearer "+fenced)
	w := httptest.NewRecorder()
	endSessionsFor(t, signer).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("ending sessions answered %d: %s", w.Code, w.Body.String())
	}

	replacement := tokenFrom(t, w)
	claims, err := signer.Verify(replacement, token.Portal)
	if err != nil {
		t.Fatalf("the replacement does not verify: %v", err)
	}

	if !claims.Enrol {
		t.Error("the replacement session is unfenced — a password alone now " +
			"reaches the whole portal with no second factor enrolled")
	}
	// And the rest of the session is unchanged, or "sign out everywhere else"
	// quietly demotes whoever pressed it.
	if claims.Role != "admin" || claims.Org != "acme" || claims.Project != "finance" {
		t.Errorf("the replacement lost something else: %+v", claims)
	}
}

// An ordinary session is not fenced by this, or every sign-out would strand
// somebody on the enrolment routes.
func TestEndingSessionsDoesNotFenceAnOrdinarySession(t *testing.T) {
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}

	ordinary, err := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "editor", Org: "acme", Project: "finance",
		Subject: "usr_2",
	}, token.MaxLifetime)
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/v1/auth/sessions/end", nil)
	r.Header.Set("Authorization", "Bearer "+ordinary)
	w := httptest.NewRecorder()
	endSessionsFor(t, signer).ServeHTTP(w, r)

	claims, err := signer.Verify(tokenFrom(t, w), token.Portal)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Enrol {
		t.Error("an ordinary session came back fenced to the enrolment routes")
	}
}

/* -- harness --------------------------------------------------------------- */

// endingRoster is a Roster that answers only the call this route makes. The
// rest of the interface is present because Go requires it, not because ending a
// session touches any of it.
type endingRoster struct{}

func (endingRoster) EndSessions(context.Context, string) (time.Time, error) {
	return time.Now().UTC(), nil
}
func (endingRoster) People(context.Context, principal.Principal) ([]identity.User, error) {
	return nil, nil
}
func (endingRoster) CreateUser(context.Context, identity.User, string) error { return nil }
func (endingRoster) SetRole(context.Context, principal.Principal, string, string) error {
	return nil
}
func (endingRoster) SetDisabled(context.Context, principal.Principal, string, bool) error {
	return nil
}
func (endingRoster) ChangePassword(context.Context, string, string, string) error { return nil }
func (endingRoster) Me(context.Context, string) (identity.User, error) {
	return identity.User{}, nil
}
func (endingRoster) SetName(context.Context, string, string) error { return nil }

func endSessionsFor(t *testing.T, signer *token.Signer) http.Handler {
	t.Helper()
	return api.NewSessions(endingRoster{}, api.NewAuthor(signer, nil), signer, quiet())
}

// tokenFrom pulls the replacement session out of the response.
func tokenFrom(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	// map[string]any, because the response also carries expiresIn as a number.
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("the response is not JSON: %s", w.Body.String())
	}
	issued, _ := out["token"].(string)
	if issued == "" {
		t.Fatalf("no replacement token in %s", w.Body.String())
	}
	return issued
}
