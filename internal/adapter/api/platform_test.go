package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Platform administration, and the line it must not cross.

The answer chosen for this deployment is administration only: a platform
administrator manages accounts, projects and tenants, and reaching a project's
data still requires membership in it. That is what makes a leaked platform
credential a control-plane problem rather than every customer's warehouse at
once.

The line is kept by principal.Principal, not by the handlers — CanRead, CanEdit,
CanAdminProject and CanAdminOrg do not mention Platform. These are the tests that
they never start to.
*/

// somebody who administers the deployment and belongs to one small project.
func platformAdmin() principal.Principal {
	return principal.Principal{
		Subject: "usr_ops", Email: "ops@cronos.example",
		OrgID: "acme", ProjectID: "finance",
		ProjectRole: principal.ProjectViewer,
		Member:      true,
		Platform:    true,
	}
}

/*
Administering the platform grants nothing inside a project.

The whole answer, in one test. If any of these four flips to true, "administration
only" has quietly become "everything", and the deployment's operator can read
every customer's numbers.
*/
func TestPlatformAdministrationGrantsNothingInsideAProject(t *testing.T) {
	pr := platformAdmin()

	// A viewer in their own project stays a viewer.
	if pr.CanEdit() {
		t.Fatal("a platform administrator can edit definitions in their own project")
	}
	if pr.CanAdminProject() {
		t.Fatal("a platform administrator administers their own project")
	}
	if pr.CanAdminOrg() {
		t.Fatal("a platform administrator administers their own organisation")
	}

	// And in a project they do not belong to, they are nobody. The project
	// resolver is what enforces this; what matters here is that the principal
	// carries no role that would let it through.
	elsewhere := pr
	elsewhere.OrgID, elsewhere.ProjectID = "globex", "ops"
	elsewhere.ProjectRole = principal.None
	if elsewhere.CanRead() {
		t.Fatal("a platform administrator reads a project they are not in")
	}
}

// And the reverse: administering a project does not administer the deployment.
func TestAProjectAdministratorIsNotAPlatformAdministrator(t *testing.T) {
	pr := principal.Principal{
		OrgID: "acme", ProjectID: "finance",
		ProjectRole: principal.ProjectAdmin, Member: true,
	}
	if pr.CanAdminPlatform() {
		t.Fatal("a project administrator administers the deployment")
	}

	// Nor does an organisation owner, who is the strongest role that existed
	// before this tier — an organisation is one customer, and the deployment is
	// all of them.
	owner := principal.Principal{OrgRole: principal.OrgOwner, OrgID: "acme", ProjectID: "finance"}
	if owner.CanAdminPlatform() {
		t.Fatal("an organisation owner administers the whole deployment")
	}
	if !owner.CanAdminProject() {
		t.Fatal("an organisation owner lost their own project")
	}
}

/*
An embed token can never claim it.

An embed token is minted by a host application for one of its own customers, so
a claim reachable from there would make every end customer of every tenant a
deployment administrator. Refused at the point claims become a principal, which
is the one place every token passes through.
*/
func TestAnEmbedTokenCannotClaimPlatformAdministration(t *testing.T) {
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}

	issued, err := signer.Mint(token.Claims{
		Audience: token.Embed, Org: "acme", Project: "finance",
		Subject: "customer-42", Role: "admin", Platform: true,
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	claims, err := signer.Verify(issued, token.Embed)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Principal().CanAdminPlatform() {
		t.Fatal("an embed token administers the deployment")
	}
	// The same refusal that already applies to the row-scope exemption.
	if claims.Principal().Member {
		t.Fatal("an embed token is a project member")
	}
}

/*
Somebody without the permission is told there is nothing there.

404 rather than 403. Everywhere else in this API a 403 is the honest answer,
because the caller is looking at their own project and already knows it exists.
Here they are not: "you may not administer the platform" confirms to somebody
probing that there is a tier to attack and an account somewhere that holds it.
*/
func TestTheseRoutesAreInvisibleWithoutThePermission(t *testing.T) {
	rows := newEmpty()
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := api.NewPlatformAPI(rows, api.NewAuthor(signer, nil), quiet())

	// A project administrator — the strongest thing short of platform.
	ordinary, err := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "admin",
		Org: "acme", Project: "finance", Subject: "usr_ada",
	}, api.SessionLifetime)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/v1/platform/tenants"},
		{http.MethodGet, "/v1/platform/people"},
		{http.MethodGet, "/v1/platform/admins"},
		{http.MethodPatch, "/v1/platform/people/usr_someone"},
		{http.MethodPost, "/v1/platform/admins/usr_someone"},
		{http.MethodDelete, "/v1/platform/admins/usr_someone"},
	} {
		r := httptest.NewRequest(c.method, c.path, strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+ordinary)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)

		if w.Code != http.StatusNotFound {
			t.Fatalf("%s %s answered %d to a project administrator", c.method, c.path, w.Code)
		}
		if strings.Contains(strings.ToLower(w.Body.String()), "platform") {
			t.Fatalf("%s %s admits the tier exists: %s", c.method, c.path, w.Body)
		}
	}
}

// And with no session at all, the same.
func TestTheseRoutesNeedASession(t *testing.T) {
	rows := newEmpty()
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := api.NewPlatformAPI(rows, api.NewAuthor(signer, nil), quiet())

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/platform/tenants", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("answered %d", w.Code)
	}
}

/*
Onboarding, which is what this tier is for.

It could move somebody between organisations, turn access off and grant
administration — and it could not create the first account of a new customer.
Onboarding meant adding the person to your own project through the ordinary
endpoint and then moving them, which is a two-step workaround for the primary
job of the whole tier.
*/
func TestAnAccountCanBeCreatedInAnyTenant(t *testing.T) {
	rows := newEmpty()
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := api.NewPlatformAPI(rows, api.NewAuthor(signer, nil), quiet())

	// An operator whose own account is in acme, creating one in globex.
	ops, err := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "viewer",
		Org: "acme", Project: "finance", Subject: "usr_ops", Platform: true,
	}, api.SessionLifetime)
	if err != nil {
		t.Fatal(err)
	}

	w := askPlatform(t, h, http.MethodPost, "/v1/platform/people", ops, map[string]string{
		"email": "dewi@globex.example", "name": "Dewi",
		"org": "globex", "project": "ops", "role": "admin",
		"password": "a-password-for-dewi",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("%d: %s", w.Code, w.Body)
	}

	rows.mu.Lock()
	defer rows.mu.Unlock()
	if len(rows.people) != 1 {
		t.Fatalf("%d accounts created", len(rows.people))
	}
	made := rows.people[0]
	if made.Org != "globex" || made.Project != "ops" || made.Role != "admin" {
		t.Fatalf("created in %s/%s as %s", made.Org, made.Project, made.Role)
	}
}

// And an ordinary project administrator cannot, which is the whole difference
// between this route and /v1/people.
func TestOnlyThisTierCanCreateAnAccountElsewhere(t *testing.T) {
	rows := newEmpty()
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := api.NewPlatformAPI(rows, api.NewAuthor(signer, nil), quiet())

	ordinary, _ := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "admin",
		Org: "acme", Project: "finance", Subject: "usr_ada",
	}, api.SessionLifetime)

	w := askPlatform(t, h, http.MethodPost, "/v1/platform/people", ordinary, map[string]string{
		"email": "dewi@globex.example", "org": "globex", "project": "ops",
		"role": "admin", "password": "a-password-for-dewi",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("a project administrator created an account elsewhere: %d", w.Code)
	}
	rows.mu.Lock()
	defer rows.mu.Unlock()
	if len(rows.people) != 0 {
		t.Fatal("an account was created")
	}
}

// A weak password is refused here too. This is the one place an account is
// created for somebody who is not present to choose their own.
func TestAnAccountCreatedElsewhereStillNeedsARealPassword(t *testing.T) {
	rows := newEmpty()
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := api.NewPlatformAPI(rows, api.NewAuthor(signer, nil), quiet())
	ops, _ := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "viewer",
		Org: "acme", Project: "finance", Subject: "usr_ops", Platform: true,
	}, api.SessionLifetime)

	for _, bad := range []map[string]string{
		{"email": "d@globex.example", "org": "globex", "project": "ops", "role": "admin", "password": "short"},
		{"email": "not-an-address", "org": "globex", "project": "ops", "role": "admin", "password": "a-password-for-dewi"},
		{"email": "d@globex.example", "org": "", "project": "ops", "role": "admin", "password": "a-password-for-dewi"},
		{"email": "d@globex.example", "org": "globex", "project": "ops", "role": "owner", "password": "a-password-for-dewi"},
	} {
		if w := askPlatform(t, h, http.MethodPost, "/v1/platform/people", ops, bad); w.Code != http.StatusBadRequest {
			t.Fatalf("%v answered %d", bad, w.Code)
		}
	}
}

// askPlatform drives one platform request.
func askPlatform(t *testing.T, h *api.PlatformAPI, method, path, session string,
	body map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	encoded, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
	r.Header.Set("Content-Type", "application/json")
	if session != "" {
		r.Header.Set("Authorization", "Bearer "+session)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
