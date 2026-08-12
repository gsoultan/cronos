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
	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
The endpoints around a second factor.

The store already guarantees that enrolment is proved and a code is spent. What
is left up here is the part where a well-implemented mechanism is still made
useless by how it is exposed:

  - a secret that comes back after enrolment, so a stolen session becomes a
    permanent second factor of the attacker's own;
  - a sign-in that says which accounts have one, so an attacker learns which
    passwords are worth buying;
  - a removal that a stolen session can perform, which strips the factor off
    the account it stole at the moment it matters most.
*/

// guarded is an in-memory Factors, enough to drive the handlers.
type guarded struct {
	mu        sync.Mutex
	secret    string
	label     string
	confirmed time.Time
	lastStep  int64
	codes     map[string]bool
	now       time.Time
	forUser   string
}

func newGuarded() *guarded {
	return &guarded{
		codes: map[string]bool{}, now: time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC),
		forUser: "usr_ada",
	}
}

func (g *guarded) mine(id string) bool { return id == g.forUser }

func (g *guarded) Enrol(_ context.Context, id, secret, label string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.mine(id) {
		return identity.ErrNoUser
	}
	if !g.confirmed.IsZero() {
		return identity.ErrFactorExists
	}
	g.secret, g.label = secret, label
	return nil
}

func (g *guarded) Enrolling(_ context.Context, id string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.mine(id) || g.secret == "" {
		return "", identity.ErrNoFactor
	}
	if !g.confirmed.IsZero() {
		return "", identity.ErrFactorExists
	}
	return g.secret, nil
}

func (g *guarded) Confirm(_ context.Context, id, code string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.mine(id) || g.secret == "" {
		return identity.ErrNoFactor
	}
	step, ok := identity.CheckTOTP(g.secret, code, g.now)
	if !ok {
		return identity.ErrBadCode
	}
	g.confirmed, g.lastStep = g.now, step
	return nil
}

func (g *guarded) FactorOf(_ context.Context, id string) (sqlstore.Factor, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.mine(id) || g.confirmed.IsZero() {
		return sqlstore.Factor{}, identity.ErrNoFactor
	}
	return sqlstore.Factor{
		Label: g.label, AddedAt: g.confirmed, Remaining: len(g.codes),
	}, nil
}

func (g *guarded) RemoveFactor(_ context.Context, id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.mine(id) || g.confirmed.IsZero() {
		return identity.ErrNoFactor
	}
	g.secret, g.confirmed, g.codes = "", time.Time{}, map[string]bool{}
	return nil
}

func (g *guarded) Protected(_ context.Context, id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.mine(id) && !g.confirmed.IsZero()
}

func (g *guarded) CheckFactor(_ context.Context, id, code string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.mine(id) || g.confirmed.IsZero() {
		return identity.ErrNoFactor
	}
	step, ok := identity.CheckTOTP(g.secret, code, g.now)
	if !ok {
		return identity.ErrBadCode
	}
	if step <= g.lastStep {
		return identity.ErrCodeUsed
	}
	g.lastStep = step
	return nil
}

func (g *guarded) SetRecoveryCodes(_ context.Context, id string, hashes []string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.mine(id) {
		return identity.ErrNoUser
	}
	g.codes = map[string]bool{}
	for _, h := range hashes {
		g.codes[h] = true
	}
	return nil
}

func (g *guarded) SpendRecoveryCode(_ context.Context, id, code string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	hash := identity.HashRecoveryCode(code)
	if !g.mine(id) || !g.codes[hash] {
		return identity.ErrBadCode
	}
	delete(g.codes, hash)
	return nil
}

func factorHandler(t *testing.T) (*guarded, *api.Factor, string) {
	t.Helper()

	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	rows := newGuarded()
	h := api.NewFactor(rows, nil, api.NewAuthor(signer, nil), "cronos", quiet())

	mine, err := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "editor",
		Org: "acme", Project: "finance", Subject: "usr_ada",
	}, api.SessionLifetime)
	if err != nil {
		t.Fatal(err)
	}
	return rows, h, mine
}

// ask drives one request against the handler.
func ask(h *api.Factor, method, path, session string, body any) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = strings.NewReader(string(encoded))
	} else {
		reader = strings.NewReader("")
	}
	r := httptest.NewRequest(method, path, reader)
	r.Header.Set("Content-Type", "application/json")
	if session != "" {
		r.Header.Set("Authorization", "Bearer "+session)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestEnrollingProducesSomethingAnAppCanScan(t *testing.T) {
	_, h, mine := factorHandler(t)

	w := ask(h, http.MethodPost, "/v1/auth/factor/start", mine, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body)
	}
	var out struct{ Secret, URI string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	if out.Secret == "" || !strings.HasPrefix(out.URI, "otpauth://totp/") {
		t.Fatalf("%+v", out)
	}
	// The URI has to encode the same secret the server stored, or the app
	// produces codes for something else and enrolment can never be confirmed.
	if !strings.Contains(out.URI, "secret="+out.Secret) {
		t.Fatalf("the QR code and the stored secret disagree: %s", out.URI)
	}
}

/*
The secret stops coming back once enrolment is confirmed.

The failure this prevents is specific: somebody with a stolen session enrols a
second factor of their own, or re-reads the secret of one already there, and
keeps a way in that survives the password being changed and the sessions being
cut.
*/
func TestTheSecretIsNotHandedBackAfterEnrolment(t *testing.T) {
	rows, h, mine := factorHandler(t)

	w := ask(h, http.MethodPost, "/v1/auth/factor/start", mine, nil)
	var started struct{ Secret string }
	_ = json.Unmarshal(w.Body.Bytes(), &started)

	code, _ := identity.TOTPCode(started.Secret, rows.now)
	if w := ask(h, http.MethodPost, "/v1/auth/factor/confirm", mine,
		map[string]string{"code": code}); w.Code != http.StatusOK {
		t.Fatalf("confirming: %d %s", w.Code, w.Body)
	}

	// Starting again is refused rather than minting a fresh secret over the
	// top of a working factor.
	if w := ask(h, http.MethodPost, "/v1/auth/factor/start", mine, nil); w.Code != http.StatusConflict {
		t.Fatalf("a second enrolment answered %d: %s", w.Code, w.Body)
	}
	// And reading the factor says what it is, not what it knows.
	shown := ask(h, http.MethodGet, "/v1/auth/factor", mine, nil)
	if strings.Contains(shown.Body.String(), started.Secret) {
		t.Fatalf("the secret came back: %s", shown.Body)
	}
}

// Confirming with a wrong code does not turn protection on — the assertion the
// wizard this replaces could not make, because it accepted any six digits.
func TestConfirmingNeedsARealCode(t *testing.T) {
	rows, h, mine := factorHandler(t)
	ask(h, http.MethodPost, "/v1/auth/factor/start", mine, nil)

	if w := ask(h, http.MethodPost, "/v1/auth/factor/confirm", mine,
		map[string]string{"code": "123456"}); w.Code == http.StatusOK {
		t.Fatal("any six digits confirmed the enrolment")
	}
	if rows.Protected(context.Background(), "usr_ada") {
		t.Fatal("a wrong code turned protection on")
	}
}

/*
Recovery codes come back once, at confirmation.

Issued then rather than at enrolment, so somebody who abandons the wizard
halfway never holds ten live credentials for an account with no second factor.
*/
func TestConfirmingIssuesRecoveryCodesOnce(t *testing.T) {
	rows, h, mine := factorHandler(t)

	w := ask(h, http.MethodPost, "/v1/auth/factor/start", mine, nil)
	var started struct{ Secret string }
	_ = json.Unmarshal(w.Body.Bytes(), &started)
	code, _ := identity.TOTPCode(started.Secret, rows.now)

	w = ask(h, http.MethodPost, "/v1/auth/factor/confirm", mine, map[string]string{"code": code})
	var out struct{ RecoveryCodes []string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.RecoveryCodes) != identity.RecoveryCodes {
		t.Fatalf("%d recovery codes", len(out.RecoveryCodes))
	}

	// And they work, which means what was shown is what was stored.
	if err := rows.SpendRecoveryCode(context.Background(), "usr_ada", out.RecoveryCodes[0]); err != nil {
		t.Fatalf("a code that was shown does not work: %v", err)
	}

	// Reading the factor afterwards does not show them again.
	shown := ask(h, http.MethodGet, "/v1/auth/factor", mine, nil)
	for _, c := range out.RecoveryCodes {
		if strings.Contains(shown.Body.String(), c) {
			t.Fatalf("a recovery code came back later: %s", shown.Body)
		}
	}
}

/*
Turning it off takes a code.

Without this, a stolen session strips the second factor off the account it
stole — at exactly the moment the factor is the only thing left. It is the one
place where making somebody prove themselves again is worth the friction.
*/
func TestRemovingAFactorNeedsProof(t *testing.T) {
	rows, h, mine := factorHandler(t)
	confirmFactor(t, rows, h, mine)

	if w := ask(h, http.MethodDelete, "/v1/auth/factor", mine,
		map[string]string{"code": ""}); w.Code == http.StatusNoContent {
		t.Fatal("a second factor was removed with no proof at all")
	}
	if w := ask(h, http.MethodDelete, "/v1/auth/factor", mine,
		map[string]string{"code": "123456"}); w.Code == http.StatusNoContent {
		t.Fatal("a second factor was removed with a guessed code")
	}
	if !rows.Protected(context.Background(), "usr_ada") {
		t.Fatal("the account lost its factor to a wrong code")
	}
}

// A recovery code removes it too, because somebody turning it off has usually
// lost the phone — and telling them to use the app they no longer have is how
// this ends with an administrator doing it over chat.
func TestARecoveryCodeCanTurnItOff(t *testing.T) {
	rows, h, mine := factorHandler(t)
	codes := confirmFactor(t, rows, h, mine)

	if w := ask(h, http.MethodDelete, "/v1/auth/factor", mine,
		map[string]string{"code": codes[0]}); w.Code != http.StatusNoContent {
		t.Fatalf("a recovery code did not turn it off: %d %s", w.Code, w.Body)
	}
	if rows.Protected(context.Background(), "usr_ada") {
		t.Fatal("still protected")
	}
}

// Somebody else's session does not reach this account's factor. Every route
// here reads the subject from the token and nothing else.
func TestAnotherSessionCannotTouchThisFactor(t *testing.T) {
	rows, h, mine := factorHandler(t)
	confirmFactor(t, rows, h, mine)

	signer, _ := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	grace, _ := signer.Mint(token.Claims{
		Audience: token.Portal, Role: "editor",
		Org: "acme", Project: "finance", Subject: "usr_grace",
	}, api.SessionLifetime)

	// Grace sees her own state — no factor — rather than ada's.
	w := ask(h, http.MethodGet, "/v1/auth/factor", grace, nil)
	if strings.Contains(w.Body.String(), `"enrolled":true`) {
		t.Fatalf("grace sees ada's factor: %s", w.Body)
	}
	// And cannot remove ada's.
	if w := ask(h, http.MethodDelete, "/v1/auth/factor", grace,
		map[string]string{"code": "123456"}); w.Code == http.StatusNoContent {
		t.Fatal("grace removed ada's second factor")
	}
	if !rows.Protected(context.Background(), "usr_ada") {
		t.Fatal("ada's factor is gone")
	}
}

// And no session at all reaches nothing.
func TestFactorRoutesNeedASession(t *testing.T) {
	_, h, _ := factorHandler(t)

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/v1/auth/factor"},
		{http.MethodPost, "/v1/auth/factor/start"},
		{http.MethodPost, "/v1/auth/factor/confirm"},
		{http.MethodPost, "/v1/auth/factor/codes"},
		{http.MethodDelete, "/v1/auth/factor"},
	} {
		if w := ask(h, c.method, c.path, "", map[string]string{"code": "123456"}); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s answered %d without a session", c.method, c.path, w.Code)
		}
	}
}

// Regenerating retires the old set, which is what somebody does after a sheet
// of codes has been photographed.
func TestRegeneratingRetiresTheOldSet(t *testing.T) {
	rows, h, mine := factorHandler(t)
	old := confirmFactor(t, rows, h, mine)

	w := ask(h, http.MethodPost, "/v1/auth/factor/codes", mine, nil)
	var out struct{ RecoveryCodes []string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	if err := rows.SpendRecoveryCode(context.Background(), "usr_ada", old[0]); !errors.Is(err, identity.ErrBadCode) {
		t.Fatal("an old code still works")
	}
	if err := rows.SpendRecoveryCode(context.Background(), "usr_ada", out.RecoveryCodes[0]); err != nil {
		t.Fatalf("a new one does not: %v", err)
	}
}

// An account with no factor has no recovery codes to generate. Ten live
// credentials for an account they do not protect is not a helpful state.
func TestRecoveryCodesNeedAFactorToRecover(t *testing.T) {
	_, h, mine := factorHandler(t)

	if w := ask(h, http.MethodPost, "/v1/auth/factor/codes", mine, nil); w.Code != http.StatusConflict {
		t.Fatalf("answered %d: %s", w.Code, w.Body)
	}
}

// confirmFactor enrols and confirms, returning the recovery codes.
func confirmFactor(t *testing.T, rows *guarded, h *api.Factor, session string) []string {
	t.Helper()

	w := ask(h, http.MethodPost, "/v1/auth/factor/start", session, nil)
	var started struct{ Secret string }
	if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil {
		t.Fatalf("enrolling: %d %s", w.Code, w.Body)
	}
	code, err := identity.TOTPCode(started.Secret, rows.now)
	if err != nil {
		t.Fatal(err)
	}

	w = ask(h, http.MethodPost, "/v1/auth/factor/confirm", session, map[string]string{"code": code})
	var out struct{ RecoveryCodes []string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("confirming: %d %s", w.Code, w.Body)
	}
	return out.RecoveryCodes
}
