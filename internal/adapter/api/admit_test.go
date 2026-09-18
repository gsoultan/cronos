package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Adding somebody who already has an account.

An email is unique across a deployment, so "that address already has an
account" used to be the end of it and working in two projects meant two
addresses. What the administrator is asking for is a membership.
*/

// takenRoster refuses every creation the way a duplicate address does.
type takenRoster struct{}

func (takenRoster) People(context.Context, principal.Principal) ([]identity.User, error) {
	return nil, nil
}
func (takenRoster) CreateUser(context.Context, identity.User, string) error {
	return identity.ErrExists
}
func (takenRoster) SetRole(context.Context, principal.Principal, string, string) error { return nil }
func (takenRoster) SetDisabled(context.Context, principal.Principal, string, bool) error {
	return nil
}
func (takenRoster) ChangePassword(context.Context, string, string, string) error { return nil }
func (takenRoster) Me(context.Context, string) (identity.User, error) {
	return identity.User{}, nil
}
func (takenRoster) SetName(context.Context, string, string) error { return nil }
func (takenRoster) EndSessions(context.Context, string) (time.Time, error) {
	return time.Time{}, nil
}

// knownAccounts answers by address and records what was granted.
type knownAccounts struct {
	users   map[string]identity.User
	granted []string
}

func (k *knownAccounts) ByEmail(_ context.Context, email string) (identity.User, error) {
	return k.users[email], nil
}
func (k *knownAccounts) GrantMembership(_ context.Context, userID, org, project, role, _ string) error {
	k.granted = append(k.granted, strings.Join([]string{userID, org, project, role}, "/"))
	return nil
}

func admitHandler(t *testing.T, accounts *knownAccounts) (http.Handler, *token.Signer) {
	t.Helper()
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	h := api.NewPeople(takenRoster{}, api.NewAuthor(signer, nil), quiet()).Admitting(accounts)
	return h, signer
}

func postPerson(t *testing.T, h http.Handler, tok, email, role string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"email":"` + email + `","name":"X","role":"` + role +
		`","password":"correct horse battery staple"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/people", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The ordinary case: an address already used in this organization becomes a
// membership in the administrator's project, with the role they asked for.
func TestAnExistingAccountIsAdmittedToThisProject(t *testing.T) {
	accounts := &knownAccounts{users: map[string]identity.User{
		"dewi@acme.example": {ID: "usr_dewi", Email: "dewi@acme.example", Org: "acme", Project: "finance"},
	}}
	h, signer := admitHandler(t, accounts)

	rec := postPerson(t, h, mint(t, signer,
		token.Claims{Org: "acme", Project: "ops", Role: "admin"}),
		"dewi@acme.example", "viewer")

	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if len(accounts.granted) != 1 || accounts.granted[0] != "usr_dewi/acme/ops/viewer" {
		t.Fatalf("granted %v, want one membership in the administrator's own project", accounts.granted)
	}
	var out identity.User
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Project != "ops" || out.Role != "viewer" {
		t.Errorf("answered with %s/%s, and the grant was ops/viewer", out.Project, out.Role)
	}
}

/*
TestAnAccountInAnotherOrganizationIsNotAdmitted is the tenancy boundary.

An administrator of one tenant's project naming another tenant's address is the
one thing this must not do. 404 and not 403, because "that is not yours" and
"no such account" are the same answer from here — telling them apart turns the
endpoint into a way to test whether an address has an account anywhere in the
deployment.
*/
func TestAnAccountInAnotherOrganizationIsNotAdmitted(t *testing.T) {
	accounts := &knownAccounts{users: map[string]identity.User{
		"spy@other.example": {ID: "usr_spy", Email: "spy@other.example", Org: "othercorp", Project: "secret"},
	}}
	h, signer := admitHandler(t, accounts)

	rec := postPerson(t, h, mint(t, signer,
		token.Claims{Org: "acme", Project: "ops", Role: "admin"}),
		"spy@other.example", "admin")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404 — it reached into another organization", rec.Code)
	}
	if len(accounts.granted) != 0 {
		t.Fatalf("and it granted %v", accounts.granted)
	}
	if strings.Contains(rec.Body.String(), "othercorp") {
		t.Error("and the refusal confirmed the other organization by name")
	}
}

// Somebody who may not manage people may not admit anybody either — the gate
// is the one the whole endpoint sits behind, and admitting is a way in.
func TestAViewerCannotAdmitAnybody(t *testing.T) {
	accounts := &knownAccounts{users: map[string]identity.User{
		"dewi@acme.example": {ID: "usr_dewi", Org: "acme", Project: "finance"},
	}}
	h, signer := admitHandler(t, accounts)

	rec := postPerson(t, h, mint(t, signer, token.Claims{Org: "acme", Project: "ops"}),
		"dewi@acme.example", "admin")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
	if len(accounts.granted) != 0 {
		t.Fatalf("and it granted %v", accounts.granted)
	}
}

// Without a records store there is no table to hold a membership, so the
// answer is the one it always was rather than a promise it cannot keep.
func TestWithoutAStoreTheAddressIsStillTaken(t *testing.T) {
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	h := api.NewPeople(takenRoster{}, api.NewAuthor(signer, nil), quiet())

	rec := postPerson(t, h, mint(t, signer,
		token.Claims{Org: "acme", Project: "ops", Role: "admin"}),
		"dewi@acme.example", "viewer")
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d, want 409", rec.Code)
	}
}
