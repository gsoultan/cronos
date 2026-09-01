package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
The first run, which is the most dangerous endpoint in the product.

For as long as it is open it hands a deployment administrator to whoever asks.
Everything worth testing is about how it closes: it must be open only while no
account exists at all, closing must survive two requests arriving together, and
nothing must reopen it.
*/

// empty is a deployment with a roster and, at first, nobody in it.
type empty struct {
	mu      sync.Mutex
	people  []identity.User
	admins  map[string]bool
	created []string
	// setUp is the marker row: one deployment is configured once.
	setUp   bool
	rekeyed []string
	adopted string
}

func newEmpty() *empty { return &empty{admins: map[string]bool{}} }

func (e *empty) CountAccounts(context.Context) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.people), nil
}

func (e *empty) SetUp(context.Context) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.setUp, nil
}

/*
FirstRun, with the marker the real store uses.

The whole point is that the check and the writes are one atomic step, so this
fake holds the lock across all of them — a fake that checked and then wrote
would pass a test the real store's transaction is there to make pass, which is
the sort of agreement that means nothing.
*/
func (e *empty) FirstRun(_ context.Context, u identity.User, _ string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.setUp {
		return sqlstore.ErrAlreadySetUp
	}
	for _, p := range e.people {
		if p.Email == u.Email {
			return identity.ErrExists
		}
	}
	e.setUp = true
	e.people = append(e.people, u)
	e.created = append(e.created, u.ID)
	e.admins[u.ID] = true
	return nil
}

func (e *empty) CreateUser(_ context.Context, u identity.User, _ string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, p := range e.people {
		if p.Email == u.Email {
			return identity.ErrExists
		}
	}
	e.people = append(e.people, u)
	e.created = append(e.created, u.ID)
	return nil
}

func (e *empty) GrantPlatform(_ context.Context, id, _ string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.admins[id] = true
	return nil
}

// The rest of the interfaces, unused here.
func (e *empty) People(context.Context, principal.Principal) ([]identity.User, error) {
	return nil, nil
}
func (e *empty) SetRole(context.Context, principal.Principal, string, string) error { return nil }
func (e *empty) SetDisabled(context.Context, principal.Principal, string, bool) error {
	return nil
}
func (e *empty) ChangePassword(context.Context, string, string, string) error { return nil }
func (e *empty) Me(context.Context, string) (identity.User, error) {
	return identity.User{}, identity.ErrNoUser
}
func (e *empty) SetName(context.Context, string, string) error { return nil }
func (e *empty) EndSessions(context.Context, string) (time.Time, error) {
	return time.Time{}, nil
}
func (e *empty) EveryPerson(context.Context) ([]identity.User, error) { return e.people, nil }
func (e *empty) Tenants(context.Context) ([]identity.Tenant, error)   { return nil, nil }

// Adopted records what the deployment was named, which is what makes a restart
// remember it. See TestAFirstRunWritesDownWhatItNamedTheDeployment.
func (e *empty) Adopted(_ context.Context, org, project string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.adopted = org + "/" + project
	return nil
}

func (e *empty) Rekey(_ context.Context, fromOrg, fromProject, toOrg, toProject string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rekeyed = append(e.rekeyed, fromOrg+"/"+fromProject+" -> "+toOrg+"/"+toProject)
	return nil
}

func (e *empty) AddPerson(_ context.Context, u identity.User, _ string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.people = append(e.people, u)
	return nil
}

func (e *empty) MovePerson(context.Context, string, string, string, string) error {
	return nil
}
func (e *empty) DisableAnywhere(context.Context, string, bool) error { return nil }
func (e *empty) PlatformAdmins(context.Context) ([]identity.User, error) {
	return nil, nil
}
func (e *empty) RevokePlatform(context.Context, string) error { return nil }

func firstRun(t *testing.T) (*empty, *api.Setup, *token.Signer) {
	t.Helper()

	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	rows := newEmpty()
	return rows, api.NewSetup(rows, signer, quiet()), signer
}

func setUp(h *api.Setup, body map[string]string) *httptest.ResponseRecorder {
	encoded, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/v1/setup", strings.NewReader(string(encoded)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func good() map[string]string {
	return map[string]string{
		"email": "ada@acme.example", "name": "Ada Rahayu",
		"password": "a-password-they-chose", "org": "Acme Logistics", "project": "Finance",
	}
}

func TestAFreshDeploymentCanBeSetUp(t *testing.T) {
	rows, h, signer := firstRun(t)

	// It says so first, which is what the portal asks before showing the page.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/setup", nil))
	if !strings.Contains(w.Body.String(), `"needed":true`) {
		t.Fatalf("a fresh deployment says %s", w.Body)
	}

	w = setUp(h, good())
	if w.Code != http.StatusCreated {
		t.Fatalf("%d: %s", w.Code, w.Body)
	}

	var out struct {
		Token string
		User  identity.User
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	// A platform administrator, and a project administrator too: platform
	// deliberately grants nothing inside a project, so the first person needs
	// both or they cannot write the first report.
	if !rows.admins[out.User.ID] {
		t.Fatal("the first account is not a platform administrator")
	}
	if out.User.Role != string(principal.ProjectAdmin) {
		t.Fatalf("the first account is a %s in its own project", out.User.Role)
	}

	claims, err := signer.Verify(out.Token, token.Portal)
	if err != nil {
		t.Fatal(err)
	}
	if !claims.Platform {
		t.Fatal("the session does not carry platform administration")
	}
	if !claims.Principal().CanAdminPlatform() {
		t.Fatal("the principal cannot administer the platform")
	}
}

/*
And then it is closed.

The property the whole endpoint exists to have. One account — any account, in
any project, administrator or not — and this is over.
*/
func TestSetupClosesTheMomentAnAccountExists(t *testing.T) {
	_, h, _ := firstRun(t)

	if w := setUp(h, good()); w.Code != http.StatusCreated {
		t.Fatalf("the first run failed: %d %s", w.Code, w.Body)
	}

	w := setUp(h, map[string]string{
		"email": "attacker@example.com", "name": "Somebody Else",
		"password": "another-password-entirely", "org": "acme", "project": "finance",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("a second setup answered %d: %s", w.Code, w.Body)
	}

	// And it says so on the way in, so the portal shows sign-in instead.
	ask := httptest.NewRecorder()
	h.ServeHTTP(ask, httptest.NewRequest(http.MethodGet, "/v1/setup", nil))
	if !strings.Contains(ask.Body.String(), `"needed":false`) {
		t.Fatalf("setup is still offered: %s", ask.Body)
	}
}

/*
Two at once produce one administrator.

Not hypothetical: it is a double-clicked button, or two people who were both
sent the URL. Checked with a read and then written, both see an empty deployment
and both proceed — and the second one is a deployment administrator nobody
meant to create.
*/
func TestTwoFirstRunsAtOnceProduceOneAdministrator(t *testing.T) {
	rows, h, _ := firstRun(t)

	const tries = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		created int
	)
	start := make(chan struct{})
	for i := range tries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			body := good()
			// Different addresses, so the unique-email check is not what saves
			// this. It is the endpoint that has to.
			body["email"] = "person" + string(rune('a'+i)) + "@acme.example"
			if setUp(h, body).Code == http.StatusCreated {
				mu.Lock()
				created++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if created != 1 {
		t.Fatalf("%d of %d concurrent first runs succeeded", created, tries)
	}
	rows.mu.Lock()
	defer rows.mu.Unlock()
	if len(rows.admins) != 1 {
		t.Fatalf("%d platform administrators exist", len(rows.admins))
	}
}

/*
The names somebody types become identifiers, so they are reduced to safe ones.

Organisation and project are half of every tenancy check in the store and end up
in file paths when definitions are kept on disk. A slash creates a directory
nobody meant; a NUL splits the key that separates the two halves; and "Acme"
being a different tenant from "acme" is a support call.
*/
func TestTypedNamesBecomeSafeIdentifiers(t *testing.T) {
	for _, c := range []struct{ typed, want string }{
		{"Acme Logistics", "acme-logistics"},
		{"  Acme   Logistics  ", "acme-logistics"},
		// A dot separates words in a name — "acme.co" reads better as "acme-co"
		// than "acmeco" — so it collapses to a hyphen like a space does. What
		// matters is that the slash is gone and nothing traversable survives.
		{"Acme/../etc", "acme-etc"},
		{"acme.co", "acme-co"},
		{"acme\x00finance", "acmefinance"},
		{"ACME", "acme"},
		{"acme_logistics", "acme-logistics"},
	} {
		rows, h, _ := firstRun(t)
		body := good()
		body["org"] = c.typed

		if w := setUp(h, body); w.Code != http.StatusCreated {
			t.Fatalf("%q was refused: %d %s", c.typed, w.Code, w.Body)
		}
		rows.mu.Lock()
		got := rows.people[0].Org
		rows.mu.Unlock()

		if got != c.want {
			t.Fatalf("%q became %q, wanted %q", c.typed, got, c.want)
		}
	}
}

/*
Nothing that could leave the directory it belongs in survives.

Definitions are kept on disk under org/project when the store is file-backed, so
a name carrying a separator is a path somebody did not mean to write. Asserted
against the result rather than against the input, because the check that matters
is what came out.
*/
func TestNoNameCanEscapeItsDirectory(t *testing.T) {
	for _, hostile := range []string{
		"../../etc/passwd", "a/b", "a\\b", ".", "..", "a\x00b", "a:b", "a%2fb",
	} {
		rows, h, _ := firstRun(t)
		body := good()
		body["org"] = hostile

		w := setUp(h, body)
		if w.Code == http.StatusBadRequest {
			// Reduced to nothing usable and refused, which is also correct.
			continue
		}
		if w.Code != http.StatusCreated {
			t.Fatalf("%q answered %d: %s", hostile, w.Code, w.Body)
		}

		rows.mu.Lock()
		got := rows.people[0].Org
		rows.mu.Unlock()

		if strings.ContainsAny(got, "/\\:%\x00.") || got == ".." || got == "" {
			t.Fatalf("%q became %q, which is a path", hostile, got)
		}
	}
}

// A name with nothing usable in it is refused rather than silently becoming an
// empty tenant, which is a tenancy check that matches everybody.
func TestANameWithNothingUsableIsRefused(t *testing.T) {
	for _, bad := range []string{"", "   ", "///", "\x00", "---"} {
		_, h, _ := firstRun(t)
		body := good()
		body["project"] = bad

		if w := setUp(h, body); w.Code != http.StatusBadRequest {
			t.Fatalf("%q was accepted as a project name: %d", bad, w.Code)
		}
	}
}

// A weak password on the account that administers everything is the one place
// it matters most.
func TestTheFirstPasswordIsHeldToTheSameRule(t *testing.T) {
	_, h, _ := firstRun(t)
	body := good()
	body["password"] = "short"

	if w := setUp(h, body); w.Code != http.StatusBadRequest {
		t.Fatalf("a five-character password set up the deployment: %d", w.Code)
	}
}

// A deployment with nowhere to keep accounts is not offered setup at all, rather
// than offered it and failing at the last step.
func TestAFileBackedDeploymentIsNotOfferedSetup(t *testing.T) {
	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))

	if api.NewSetup(nil, signer, quiet()).Available() {
		t.Fatal("a deployment with no store offers setup")
	}
	rows := newEmpty()
	if !api.NewSetup(rows, signer, quiet()).Available() {
		t.Fatal("a deployment with a store does not")
	}
}

// A body that names more than this endpoint takes is refused rather than having
// the extra fields ignored — the same rule as accepting an invitation.
func TestTheFirstRunBodyIsStrict(t *testing.T) {
	for _, extra := range []string{"role", "platform", "id", "disabled"} {
		_, h, _ := firstRun(t)
		body := good()
		body[extra] = "admin"

		if w := setUp(h, body); w.Code != http.StatusBadRequest {
			t.Fatalf("a body naming %q was accepted: %d %s", extra, w.Code, w.Body)
		}
	}
}

/*
A first run writes down what it named the deployment.

The adoption was in memory and nowhere else, so the deployment forgot it on the
next restart: the process came back believing it served whatever CRONOS_ORG
said, found an empty store for that tenant, adopted the definitions directory a
second time under the old name, and answered "you do not have access to this
project" to the administrator who had set it up ten seconds earlier.

Every deployment set up through the browser broke on its first restart. Nothing
caught it because nothing ever restarted one — every check in this repository
sets a deployment up and then uses it, which is exactly the window in which it
works.
*/
func TestAFirstRunWritesDownWhatItNamedTheDeployment(t *testing.T) {
	rows, h, _ := firstRun(t)
	h = h.Serving(&api.One{Org: "default", ProjectID: "default", Only: &api.Project{}})

	w := setUp(h, map[string]string{
		"email": "ada@acme.example", "name": "Ada",
		"password": "an-administrators-password",
		"org":      "Acme", "project": "Finance",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("%d: %s", w.Code, w.Body)
	}

	rows.mu.Lock()
	defer rows.mu.Unlock()
	if rows.adopted != "acme/finance" {
		t.Fatalf("the deployment recorded %q as its name, so a restart will not remember it",
			rows.adopted)
	}
}
