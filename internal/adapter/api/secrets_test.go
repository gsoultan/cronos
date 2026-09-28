package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/app/vault"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/secret"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// secretRows is a store in a map, keyed as the real one is.
type secretRows map[string]vault.Stored

func (m secretRows) Sealed(_ context.Context, org, project, name string) ([]byte, bool, error) {
	st, ok := m[org+"/"+project+"/"+name]
	return st.Sealed, ok, nil
}

func (m secretRows) SecretsOf(_ context.Context, org, project string) ([]vault.Stored, error) {
	var out []vault.Stored
	for _, st := range m {
		if st.Org == org && st.Project == project {
			out = append(out, st)
		}
	}
	return out, nil
}

func (m secretRows) PutSecret(_ context.Context, st vault.Stored) error {
	m[st.Org+"/"+st.Project+"/"+st.Name] = st
	return nil
}

func (m secretRows) DeleteSecret(_ context.Context, org, project, name string) (bool, error) {
	_, ok := m[org+"/"+project+"/"+name]
	delete(m, org+"/"+project+"/"+name)
	return ok, nil
}

// oneVault answers for acme/finance and nothing else — the resolver boot
// builds does the same through api.Projects.
type oneVault struct{ v *vault.Service }

func (o oneVault) Storing() bool { return o.v.Available() }
func (o oneVault) List(ctx context.Context, pr principal.Principal) ([]vault.Entry, error) {
	return o.v.List(ctx, pr)
}
func (o oneVault) Set(ctx context.Context, pr principal.Principal, name, value string) error {
	return o.v.Set(ctx, pr, name, value)
}
func (o oneVault) Delete(ctx context.Context, pr principal.Principal, name string) error {
	return o.v.Delete(ctx, pr, name)
}

// nobodyPublishes is enough of a publisher for the management routes to mount.
type nobodyPublishes struct{}

func (nobodyPublishes) Publish(context.Context, []byte, principal.Principal) (publish.Result, error) {
	return publish.Result{}, publish.ErrForbidden
}
func (nobodyPublishes) PublishIf(context.Context, []byte, principal.Principal, string) (publish.Result, error) {
	return publish.Result{}, publish.ErrForbidden
}
func (nobodyPublishes) Delete(context.Context, principal.Principal, string, string) error {
	return publish.ErrForbidden
}

type secretsServer struct {
	h      http.Handler
	signer *token.Signer
	rows   secretRows
	logs   *bytes.Buffer
}

func newSecretsServer(t *testing.T, sealing bool) secretsServer {
	t.Helper()
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	var seal vault.Sealer
	if sealing {
		s, err := secret.NewSealer([]byte("a-secrets-key-that-is-long-enough-to-seal"))
		if err != nil {
			t.Fatal(err)
		}
		seal = s
	}
	rows := secretRows{}
	v := vault.New(rows, seal, func() (string, string) { return "acme", "finance" }, nil)
	logs := &bytes.Buffer{}
	h := api.Routes(api.Deps{
		Signer: signer, Log: logger(logs), Origins: []string{"http://localhost:5174"},
		Publish: nobodyPublishes{}, Secrets: oneVault{v},
	})
	return secretsServer{h: h, signer: signer, rows: rows, logs: logs}
}

func (s secretsServer) do(t *testing.T, role, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	tok, err := s.signer.Mint(token.Claims{Audience: token.Portal, Role: role,
		Org: "acme", Project: "finance", Subject: "usr_1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	s.h.ServeHTTP(w, r)
	return w
}

// A secret goes in and never comes out: not in the list, not in the log.
func TestASecretIsWrittenAndNeverReadBack(t *testing.T) {
	s := newSecretsServer(t, true)

	w := s.do(t, "editor", http.MethodPut, "/v1/secrets/mapbox-token", `{"value":"pk.the-real-token"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("PUT answered %d: %s", w.Code, w.Body)
	}
	w = s.do(t, "editor", http.MethodGet, "/v1/secrets", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET answered %d: %s", w.Code, w.Body)
	}
	var got struct {
		Store   bool          `json:"store"`
		Secrets []vault.Entry `json:"secrets"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Store || len(got.Secrets) != 1 || got.Secrets[0].Name != "mapbox-token" ||
		got.Secrets[0].Source != vault.FromProject {
		t.Errorf("listed %+v", got)
	}
	for where, text := range map[string]string{"the list": w.Body.String(), "the log": s.logs.String()} {
		if strings.Contains(text, "pk.the-real-token") {
			t.Errorf("the value is in %s", where)
		}
	}
	for _, st := range s.rows {
		if bytes.Contains(st.Sealed, []byte("pk.the-real-token")) {
			t.Error("the store holds the value as it was typed")
		}
	}
}

func TestAViewerManagesNoSecrets(t *testing.T) {
	s := newSecretsServer(t, true)
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/secrets", ""},
		{http.MethodPut, "/v1/secrets/mapbox-token", `{"value":"pk.x"}`},
		{http.MethodDelete, "/v1/secrets/mapbox-token", ""},
	} {
		if w := s.do(t, "viewer", c.method, c.path, c.body); w.Code != http.StatusForbidden {
			t.Errorf("a viewer's %s %s answered %d", c.method, c.path, w.Code)
		}
	}
	if len(s.rows) != 0 {
		t.Errorf("a viewer stored %d secrets", len(s.rows))
	}
}

// Without a key the list still answers — it is where somebody learns what to
// set — and a write says what is missing rather than failing somewhere else.
func TestWithoutAKeyTheAPISaysWhatToSet(t *testing.T) {
	s := newSecretsServer(t, false)

	w := s.do(t, "editor", http.MethodGet, "/v1/secrets", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"store":false`) {
		t.Errorf("GET without a key answered %d: %s", w.Code, w.Body)
	}
	w = s.do(t, "editor", http.MethodPut, "/v1/secrets/mapbox-token", `{"value":"pk.x"}`)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "CRONOS_SECRETS_KEY") {
		t.Errorf("PUT without a key answered %d: %s", w.Code, w.Body)
	}
}

func TestABadSecretIsRefusedWithTheReason(t *testing.T) {
	s := newSecretsServer(t, true)
	w := s.do(t, "editor", http.MethodPut, "/v1/secrets/has%20space", `{"value":"x"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("a name with a space answered %d: %s", w.Code, w.Body)
	}
	w = s.do(t, "editor", http.MethodPut, "/v1/secrets/ok", `{"password":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("a body with the wrong field answered %d: %s", w.Code, w.Body)
	}
	w = s.do(t, "editor", http.MethodDelete, "/v1/secrets/never-set", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("deleting what is not there answered %d: %s", w.Code, w.Body)
	}
}
