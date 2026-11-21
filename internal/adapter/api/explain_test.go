package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/explain"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/query"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
An explanation sends a reader's figures to somebody else's model, so the one
property worth proving is that the figures are the reader's: a viewer confined
to one customer gets an explanation built from that customer's rows, and the
other customer's never reach the model at all.
*/

// confined is an account held to one customer, as an administrator set it.
type confined struct {
	*standing
	scope map[string]map[string]string
}

func (c *confined) Confinement(_ context.Context, _, _, user string) (map[string]string, error) {
	return c.scope[user], nil
}

// model records every prompt it is given and answers each the same way.
type model struct {
	mu      sync.Mutex
	prompts []explain.Prompt
}

func (m *model) Complete(_ context.Context, p explain.Prompt) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prompts = append(m.prompts, p)
	return "Revenue is what customers paid.", nil
}

func (m *model) sent() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	for _, p := range m.prompts {
		b.WriteString(p.User)
	}
	return b.String()
}

func explainServer(t *testing.T, explains api.Explaining) (http.Handler, *token.Signer) {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:explain%d?mode=memory&cache=shared", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`
CREATE TABLE sales (customer TEXT, amount REAL);
INSERT INTO sales VALUES ('acme', 1200), ('acme', 34), ('globex', 987654);`); err != nil {
		t.Fatal(err)
	}
	ds, err := yamlcodec.Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: sales}
spec:
  sources: [{ref: warehouse}]
  query: SELECT customer, amount FROM sales
  fields:
    - {name: customer, type: string,  role: dimension}
    - {name: amount,   type: decimal, role: measure, aggregate: sum}
  rowLevelSecurity:
    - predicate: customer = {{ .scope.customer }}`))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: revenue}
spec:
  dataset: sales
  outputs:
    - name: screen
      renderer: interactive
      layout:
        - kind: chart
          chart: bar
          title: Revenue by customer
          x: {field: customer}
          y: {field: amount, aggregate: sum}`))
	if err != nil {
		t.Fatal(err)
	}
	runner := run.New(fixedDatasets{"sales": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
	}})
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	roster := &confined{
		standing: &standing{known: true, active: true, now: time.Now},
		scope:    map[string]map[string]string{"usr_acme": {"customer": "acme"}},
	}
	one := &api.One{Org: "acme-isv", ProjectID: "sales",
		Only: &api.Project{Reports: oneReport{rep}, Runner: runner}}
	return api.Routes(api.Deps{Projects: one, Signer: signer, Roster: roster,
		Explains: explains, Log: logger(&bytes.Buffer{})}), signer
}

func viewerToken(t *testing.T, s *token.Signer, subject string) string {
	t.Helper()
	tok, err := s.Mint(token.Claims{Audience: token.Portal, Role: "viewer",
		Org: "acme-isv", Project: "sales", Subject: subject}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAnExplanationReadsOnlyTheRowsItsReaderMaySee(t *testing.T) {
	m := &model{}
	h, s := explainServer(t, explain.New(m))

	w := postJSON(h, "/v1/reports/revenue/explain", viewerToken(t, s, "usr_acme"), map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "Revenue is what customers paid.") {
		t.Fatalf("the answer was not returned: %s", w.Body)
	}
	sent := m.sent()
	if !strings.Contains(sent, "acme") || !strings.Contains(sent, "1,234") {
		t.Fatalf("the reader's own figures were not sent:\n%s", sent)
	}
	for _, leak := range []string{"globex", "987"} {
		if strings.Contains(sent, leak) {
			t.Fatalf("another customer's %q reached the model:\n%s", leak, sent)
		}
	}
}

func TestABlockThatIsNotThereIsTheReadersToFix(t *testing.T) {
	m := &model{}
	h, s := explainServer(t, explain.New(m))

	w := postJSON(h, "/v1/reports/revenue/explain", viewerToken(t, s, "usr_acme"), map[string]any{"block": 7})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "no block 7") {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if len(m.prompts) != 0 {
		t.Fatal("a refused request still reached the model")
	}
}

func TestAnExplanationNeedsASession(t *testing.T) {
	h, _ := explainServer(t, explain.New(&model{}))
	if w := postJSON(h, "/v1/reports/revenue/explain", "", map[string]any{}); w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
}

// Without a model there is no route — not a route that always says no.
func TestNothingIsExplainedWithoutAModel(t *testing.T) {
	h, s := explainServer(t, nil)
	w := postJSON(h, "/v1/reports/revenue/explain", viewerToken(t, s, "usr_acme"), map[string]any{})
	if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
}
