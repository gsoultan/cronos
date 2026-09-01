package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
The invitation endpoints, and the one asymmetry that matters.

Sending needs an administrator's session. Accepting needs none — it cannot,
because the person using it does not have an account yet, which is the entire
point. So every property that endpoint needs is a property of the secret, and
these are the tests that it has them.
*/

// held is an in-memory invitation store, enough to drive the handlers.
type held struct {
	mu    sync.Mutex
	rows  map[string]*identity.Invitation // by hash
	users map[string]bool                 // emails that already have accounts
	now   time.Time
}

func newHeld() *held {
	return &held{
		rows: map[string]*identity.Invitation{}, users: map[string]bool{},
		now: time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC),
	}
}

func (h *held) Invite(_ context.Context, inv identity.Invitation, hash string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.users[inv.Email] {
		return identity.ErrExists
	}
	inv.CreatedAt = h.now
	h.rows[hash] = &inv
	return nil
}

func (h *held) find(secret string) (*identity.Invitation, error) {
	row, ok := h.rows[identity.HashInvitation(secret)]
	if !ok || !row.Usable(h.now) {
		return nil, identity.ErrInvitation
	}
	return row, nil
}

func (h *held) Invitation(_ context.Context, secret string) (identity.Invitation, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	row, err := h.find(secret)
	if err != nil {
		return identity.Invitation{}, err
	}
	return *row, nil
}

func (h *held) Accept(_ context.Context, secret, _ string) (identity.User, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	row, err := h.find(secret)
	if err != nil {
		return identity.User{}, err
	}
	at := h.now
	row.Accepted = &at

	h.users[row.Email] = true
	return identity.User{
		ID: identity.NewID(), Email: row.Email, Name: row.Name,
		Org: row.Org, Project: row.Project, Role: row.Role,
	}, nil
}

func (h *held) Invitations(_ context.Context, pr principal.Principal) ([]identity.Invitation, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := []identity.Invitation{}
	for _, row := range h.rows {
		if row.Org == pr.OrgID && row.Project == pr.ProjectID && row.Usable(h.now) {
			out = append(out, *row)
		}
	}
	return out, nil
}

func (h *held) Uninvite(_ context.Context, pr principal.Principal, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	for hash, row := range h.rows {
		if row.ID == id && row.Org == pr.OrgID && row.Project == pr.ProjectID {
			delete(h.rows, hash)
			return nil
		}
	}
	return identity.ErrInvitation
}

// posted is a mail server that keeps what it was given.
type posted struct {
	mu   sync.Mutex
	sent []struct{ To, Subject, Body string }
	fail error
}

func (p *posted) Post(_ context.Context, to, subject, body string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.fail != nil {
		return p.fail
	}
	p.sent = append(p.sent, struct{ To, Subject, Body string }{to, subject, body})
	return nil
}

func (p *posted) last() (to, subject, body string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.sent) == 0 {
		return "", "", ""
	}
	l := p.sent[len(p.sent)-1]
	return l.To, l.Subject, l.Body
}

// secretFrom pulls the link's fragment out of an email, the way the person
// clicking it would.
func secretFrom(t *testing.T, body string) string {
	t.Helper()

	_, after, ok := strings.Cut(body, "#secret=")
	if !ok {
		t.Fatalf("no link in the email:\n%s", body)
	}
	return strings.Fields(after)[0]
}

func inviter(t *testing.T) (*api.Invite, *held, *posted) {
	t.Helper()

	rows, mail := newHeld(), &posted{}
	return api.NewInvite(rows, mail, "https://reports.acme.example", quiet()), rows, mail
}

func admin() principal.Principal {
	return principal.Principal{
		Subject: "usr_ada", Email: "ada@acme.example",
		OrgID: "acme", ProjectID: "finance", ProjectRole: principal.ProjectAdmin,
	}
}

func TestAnInvitationIsMailedAndOpensAnAccount(t *testing.T) {
	invite, rows, mail := inviter(t)

	if _, err := invite.Send(context.Background(), admin(), "ada@acme.example",
		"dewi@acme.example", "Dewi", "editor"); err != nil {
		t.Fatal(err)
	}

	to, subject, body := mail.last()
	if to != "dewi@acme.example" {
		t.Fatalf("sent to %q", to)
	}
	if !strings.Contains(subject, "finance") {
		t.Fatalf("the subject does not say where: %q", subject)
	}
	// Named, and by whom. A link from nobody, addressed to nobody, is what a
	// phishing email looks like, and the recipient's only real check is
	// whether it reads like a person they know wrote it.
	if !strings.Contains(body, "Dewi") || !strings.Contains(body, "ada@acme.example") {
		t.Fatalf("the email says neither who it is for nor who sent it:\n%s", body)
	}

	// The secret in the email opens the invitation.
	secret := secretFrom(t, body)
	if _, err := rows.Invitation(context.Background(), secret); err != nil {
		t.Fatalf("the link in the email does not work: %v", err)
	}
}

/*
The secret is in the fragment, not the query.

A browser does not send anything after `#` to any server. In the query string
the secret is in the portal's access log, its CDN's log, and the Referer header
of whatever the page loads next — three copies of a working credential, made by
choosing the wrong punctuation.
*/
func TestTheSecretIsInTheFragment(t *testing.T) {
	invite, _, mail := inviter(t)
	if _, err := invite.Send(context.Background(), admin(), "ada@acme.example",
		"dewi@acme.example", "Dewi", "editor"); err != nil {
		t.Fatal(err)
	}

	_, _, body := mail.last()
	link := "https://reports.acme.example/invitation#secret="
	if !strings.Contains(body, link) {
		t.Fatalf("the link is not a fragment handoff:\n%s", body)
	}

	// And nothing before the `#` carries it.
	secret := secretFrom(t, body)
	before, _, _ := strings.Cut(body[strings.Index(body, link):], "#")
	if strings.Contains(before, secret) {
		t.Fatalf("the secret is in the path or query: %q", before)
	}
}

/*
Where somebody lands comes from the invitation, never from the request.

The accept endpoint has no session, so if the organisation, project or role came
out of the body, anybody with any invitation could join any project as an
administrator by editing one JSON field.

Two answers, and both are asserted. A body that tries to say more than a secret
and a password is refused outright rather than having the extra fields ignored —
strict decoding, so a field added to this struct later cannot silently become
attacker-controlled. And a well-formed one lands exactly where the invitation
said.
*/
func TestAcceptingCannotChooseTheProjectOrRole(t *testing.T) {
	invite, rows, mail := inviter(t)
	if _, err := invite.Send(context.Background(), admin(), "ada@acme.example",
		"dewi@acme.example", "Dewi", "viewer"); err != nil {
		t.Fatal(err)
	}
	_, _, body := mail.last()

	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	h := api.NewAcceptance(rows, signer, quiet())

	// Anything beyond the two fields this endpoint takes is refused.
	for _, extra := range []string{"org", "project", "role", "email", "id"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, jsonPost("/v1/auth/invitation", map[string]string{
			"secret": secretFrom(t, body), "password": "a-password-they-chose",
			extra: "globex",
		}))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("a body naming %q was accepted: %d %s", extra, w.Code, w.Body)
		}
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, jsonPost("/v1/auth/invitation", map[string]string{
		"secret": secretFrom(t, body), "password": "a-password-they-chose",
	}))

	if w.Code != http.StatusCreated {
		t.Fatalf("accepting answered %d: %s", w.Code, w.Body)
	}
	var out struct {
		Token string
		User  identity.User
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	if out.User.Org != "acme" || out.User.Project != "finance" {
		t.Fatalf("landed in %s/%s", out.User.Org, out.User.Project)
	}
	if out.User.Role != "viewer" {
		t.Fatalf("arrived as %s", out.User.Role)
	}
	if out.User.Email != "dewi@acme.example" {
		t.Fatalf("arrived as %s", out.User.Email)
	}

	// And the session they were handed says the same thing. A body that
	// changed the token's claims but not the stored user would be the real
	// hole, since the token is what every later request is checked against.
	claims, err := signer.Verify(out.Token, token.Portal)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Org != "acme" || claims.Project != "finance" || claims.Role != "viewer" {
		t.Fatalf("the session says %s/%s as %s", claims.Org, claims.Project, claims.Role)
	}
}

// Unusable for any reason answers the same. Telling "expired" from "spent" from
// "never existed" on an endpoint with no session is a way to learn which
// addresses somebody is onboarding.
func TestEveryUnusableInvitationAnswersAlike(t *testing.T) {
	invite, rows, mail := inviter(t)
	if _, err := invite.Send(context.Background(), admin(), "ada@acme.example",
		"dewi@acme.example", "Dewi", "editor"); err != nil {
		t.Fatal(err)
	}
	_, _, body := mail.last()
	secret := secretFrom(t, body)

	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := api.NewAcceptance(rows, signer, quiet())

	if _, err := rows.Accept(context.Background(), secret, "a-password-they-chose"); err != nil {
		t.Fatal(err)
	}

	answers := map[string]string{}
	for _, which := range []string{secret, "a-secret-nobody-issued", ""} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
			"/v1/auth/invitation?secret="+which, nil))
		answers[which] = http.StatusText(w.Code) + " " + w.Body.String()
	}

	seen := map[string]bool{}
	for _, answer := range answers {
		seen[answer] = true
	}
	if len(seen) != 1 {
		t.Fatalf("spent, unknown and empty answer differently: %v", answers)
	}
}

// A person who already has an account is not invited: the link would create a
// second account for one address, or fail at the very end after they had
// already chosen a password.
func TestInvitingSomebodyWhoAlreadyHasAnAccount(t *testing.T) {
	invite, rows, mail := inviter(t)
	rows.users["dewi@acme.example"] = true

	_, err := invite.Send(context.Background(), admin(), "ada@acme.example", "dewi@acme.example", "Dewi", "editor")
	if !errors.Is(err, identity.ErrExists) {
		t.Fatalf("invited anyway: %v", err)
	}
	if to, _, _ := mail.last(); to != "" {
		t.Fatalf("an email went to %s", to)
	}
}

/*
Mail that could not be sent is reported as mail that could not be sent.

The invitation is written first and then sent, so a relay that is down leaves a
row nobody has the secret for — which expires on its own. What must not happen
is the administrator being told it worked.
*/
func TestAnUndeliveredInvitationSaysSo(t *testing.T) {
	invite, _, mail := inviter(t)
	mail.fail = errors.New("connection refused")

	inv, err := invite.Send(context.Background(), admin(), "ada@acme.example", "dewi@acme.example", "Dewi", "editor")
	if err == nil {
		t.Fatal("a failed send was reported as a success")
	}
	// And it says which half worked, because "check the mail server" and "try
	// again" are different instructions.
	if inv.ID == "" {
		t.Fatal("the caller cannot tell whether the invitation was written")
	}
	if !strings.Contains(err.Error(), "not sent") {
		t.Fatalf("the error does not say what failed: %v", err)
	}
}

/*
No mail server, no invitations.

A deployment that cannot send email must not offer a button that produces an
error somebody has to interpret — and must not quietly create the account with
a password nobody chose, which is the same failure this feature exists to
remove.
*/
func TestADeploymentWithoutMailCannotInvite(t *testing.T) {
	if api.NewInvite(newHeld(), nil, "https://reports.acme.example", quiet()).Available() {
		t.Fatal("a deployment with no mail server offers invitations")
	}
	// Nor one that has mail but nowhere to send anybody, which would produce a
	// link with no host in it.
	if api.NewInvite(newHeld(), &posted{}, "", quiet()).Available() {
		t.Fatal("a deployment with no portal URL offers invitations")
	}
	if !api.NewInvite(newHeld(), &posted{}, "https://reports.acme.example", quiet()).Available() {
		t.Fatal("a deployment with both does not")
	}
}

// Describing an invitation says who it is for and nothing about who else is
// there. It is read by somebody with no session, and "the finance project has
// 40 people" is not theirs to learn.
func TestDescribingAnInvitationSaysLittle(t *testing.T) {
	invite, rows, mail := inviter(t)
	if _, err := invite.Send(context.Background(), admin(), "ada@acme.example",
		"dewi@acme.example", "Dewi", "editor"); err != nil {
		t.Fatal(err)
	}
	_, _, body := mail.last()

	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	h := api.NewAcceptance(rows, signer, quiet())

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/v1/auth/invitation?secret="+secretFrom(t, body), nil))

	if w.Code != http.StatusOK {
		t.Fatalf("answered %d: %s", w.Code, w.Body)
	}
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	for _, field := range []string{"email", "project", "role", "invitedBy"} {
		if out[field] == "" {
			t.Fatalf("nothing to show the person: %v", out)
		}
	}
	// The secret is not echoed back. It arrived in a URL; putting it in a
	// response body puts it somewhere a script can read it.
	for _, v := range out {
		if strings.Contains(v, secretFrom(t, body)) {
			t.Fatalf("the secret came back in the answer: %v", out)
		}
	}
}

// jsonPost builds a request with a JSON body.
func jsonPost(path string, body map[string]string) *http.Request {
	encoded, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(encoded)))
	r.Header.Set("Content-Type", "application/json")
	return r
}
