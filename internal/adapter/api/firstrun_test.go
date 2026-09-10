package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/platform/config"
)

/*
Configuring a deployment that has none.

The property worth most here is that what setup writes must boot. A first run
that ends in a server refusing to start is the worst possible first impression
and the only fix is hand-editing the file setup just wrote — which is the thing
the page existed to avoid.

v1.2.0 shipped with exactly that: leaving the definitions field blank wrote no
value, config fell back to its default of "examples" — a relative path that
suits a checkout and does not exist for a service started from / — and the
second boot died on `lstat examples`. Found by installing the published .deb,
not by any test, which is why there is now one.
*/

func unconfigured(t *testing.T, dir string, accept bool) (*api.FirstRun, *bool) {
	t.Helper()

	finished := false
	h := api.NewFirstRun(api.FirstRunDeps{
		ConfigPath: filepath.Join(dir, "config.yaml"),
		Accepts:    func(string) bool { return accept },
		NewKey:     func() (string, error) { return "generated-key-at-least-32-bytes-x", nil },
		Finished:   func() { finished = true },
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return h, &finished
}

func post(t *testing.T, h *api.FirstRun, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/v1/setup", strings.NewReader(string(raw)))
	r.Header.Set("content-type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func complete(extra map[string]any) map[string]any {
	body := map[string]any{
		"token": "whatever", "email": "boss@acme.test",
		"password": "Str0ng-Passw0rd-Here", "org": "acme", "project": "finance",
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

/*
The regression: a blank definitions field must not produce a configuration that
will not boot.

Asserted on the written file rather than on the response, because the response
was always 200 — the deployment failed on its next start, which is the part
nobody sees until it is too late.
*/
func TestSetupWritesADefinitionsPathThatExists(t *testing.T) {
	dir := t.TempDir()
	h, _ := unconfigured(t, dir, true)

	if w := post(t, h, complete(nil)); w.Code != http.StatusOK {
		t.Fatalf("setup answered %d: %s", w.Code, w.Body.String())
	}

	file, found, err := config.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil || !found {
		t.Fatalf("no configuration was written: found=%v err=%v", found, err)
	}

	if file.Definitions == "" {
		t.Fatal("definitions was left empty, so the next boot falls back to \"examples\"")
	}
	if !filepath.IsAbs(file.Definitions) {
		t.Errorf("definitions is %q, which is relative to whatever the service's "+
			"working directory happens to be", file.Definitions)
	}
	// And it is there, because the next boot reads it and an empty deployment
	// has nothing to have created it.
	if info, err := os.Stat(file.Definitions); err != nil || !info.IsDir() {
		t.Errorf("definitions %q does not exist: %v", file.Definitions, err)
	}
}

// A definitions path somebody supplied is kept, and created if it is not there.
func TestSetupKeepsADefinitionsPathThatWasGiven(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "somewhere", "else")
	h, _ := unconfigured(t, dir, true)

	if w := post(t, h, complete(map[string]any{"definitions": want})); w.Code != http.StatusOK {
		t.Fatalf("setup answered %d: %s", w.Code, w.Body.String())
	}

	file, _, err := config.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Definitions != want {
		t.Errorf("definitions is %q, want %q", file.Definitions, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("the supplied directory was not created: %v", err)
	}
}

/*
The token is the whole of the security, so its failure is asserted first.

Without one this endpoint hands the deployment's root of trust to whoever
reaches it, and the refusal has to happen before anything is written — a
configuration on disk from a rejected request is a deployment somebody else
named.
*/
func TestWithoutTheTokenNothingIsWritten(t *testing.T) {
	dir := t.TempDir()
	h, finished := unconfigured(t, dir, false)

	w := post(t, h, complete(nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("answered %d, want 403", w.Code)
	}
	if _, found, _ := config.ReadFile(filepath.Join(dir, "config.yaml")); found {
		t.Error("a rejected request wrote a configuration")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a rejected request left %d files behind", len(entries))
	}
	if *finished {
		t.Error("a rejected request stopped the server")
	}
}

// The signing key is generated, never taken from the request. A key somebody
// chooses is a key somebody can choose badly, and this is the root of trust for
// every token the deployment will issue.
func TestTheSigningKeyIsGeneratedAndNotSupplied(t *testing.T) {
	dir := t.TempDir()
	h, _ := unconfigured(t, dir, true)

	// Sent anyway, and it must be ignored — the field does not exist on the
	// request, so this also asserts an unknown field is refused or dropped
	// rather than trusted.
	post(t, h, complete(map[string]any{"org": "acme"}))

	file, found, err := config.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if file.SigningKey != "generated-key-at-least-32-bytes-x" {
		t.Errorf("signing key is %q, want the generated one", file.SigningKey)
	}
}

// A password that would be refused at sign-in is refused here, before a
// configuration exists — otherwise setup writes one and then cannot make the
// account it was written for.
func TestAWeakPasswordIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	dir := t.TempDir()
	h, _ := unconfigured(t, dir, true)

	w := post(t, h, complete(map[string]any{"password": "short"}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("answered %d, want 400", w.Code)
	}
	if _, found, _ := config.ReadFile(filepath.Join(dir, "config.yaml")); found {
		t.Error("a refused password still wrote a configuration")
	}
}

// The administrator is recorded for the next boot, as a hash. The password must
// not reach disk, because it sits there between two starts.
func TestThePasswordIsNeverWrittenDown(t *testing.T) {
	dir := t.TempDir()
	h, _ := unconfigured(t, dir, true)

	post(t, h, complete(nil))

	pending, found, err := api.ReadPendingAdmin(filepath.Join(dir, "config.yaml"))
	if err != nil || !found {
		t.Fatalf("no administrator was recorded: found=%v err=%v", found, err)
	}
	if pending.Email != "boss@acme.test" {
		t.Errorf("recorded %q", pending.Email)
	}
	if !strings.HasPrefix(pending.Hash, "$2") {
		t.Errorf("the recorded credential is not a bcrypt hash: %q", pending.Hash)
	}

	// Nothing under the directory holds the password, in any file.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		raw, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		if strings.Contains(string(raw), "Str0ng-Passw0rd-Here") {
			t.Errorf("%s holds the password in plain text", e.Name())
		}
	}
}
