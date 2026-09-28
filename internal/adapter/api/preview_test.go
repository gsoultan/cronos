package api_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/query"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
A draft drawn in the builder: for somebody who may publish it, refused where
publishing it would be and with the same sentence, and never for a reader, an
embedded page, or anybody from another project — who must not learn so much as
which datasets exist here from the way a draft of theirs is refused.
*/

// previewServer is a project of three drops and the server over it.
func previewServer(t *testing.T) (http.Handler, *token.Signer) {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:preview%d?mode=memory&cache=shared", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE drops (id TEXT, lat REAL, lon REAL, parcels REAL);
INSERT INTO drops VALUES ('d1', 51.50, -0.12, 3), ('d2', 51.51, -0.13, 1), ('d3', 51.52, -0.11, 2);`); err != nil {
		t.Fatal(err)
	}
	ds, err := yamlcodec.Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: drops}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, lat, lon, parcels FROM drops
  fields:
    - {name: id,      type: string,  role: dimension}
    - {name: lat,     type: decimal, role: dimension}
    - {name: lon,     type: decimal, role: dimension}
    - {name: parcels, type: decimal, role: measure, aggregate: sum}`))
	if err != nil {
		t.Fatal(err)
	}
	sets := fixedDatasets{"drops": ds}
	runner := run.New(sets, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
	}})
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	one := &api.One{Org: "acme", ProjectID: "maps", Only: &api.Project{Reports: oneReport{}, Runner: runner}}
	return api.Routes(api.Deps{
		Projects: one, Signer: signer, Publish: publish.New(nil, sets), Log: logger(&bytes.Buffer{}),
	}), signer
}

// draft is a report of one map block over dataset, drawing layers.
func draft(dataset, layers string) string {
	return `apiVersion: cronos.dev/v1
kind: Report
metadata: {name: draft}
spec:
  dataset: ` + dataset + `
  outputs:
    - name: screen
      renderer: interactive
      layout:
        - kind: chart
          chart: map
          title: Drops
          x: {field: id}
          y: {field: parcels, aggregate: sum}
          map: {layers: [` + layers + `], lat: lat, lon: lon}`
}

// preview posts a draft as whoever tok is, and returns the status and body.
func preview(t *testing.T, h http.Handler, tok, report string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"report": report})
	r := httptest.NewRequest(http.MethodPost, "/v1/preview", bytes.NewReader(body))
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

func portal(t *testing.T, s *token.Signer, org, role string) string {
	t.Helper()
	tok, err := s.Mint(token.Claims{Audience: token.Portal, Role: role, Org: org, Project: "maps",
		Subject: "usr_" + role}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAnEditorPreviewsADraft(t *testing.T) {
	h, s := previewServer(t)
	code, body := preview(t, h, portal(t, s, "acme", "editor"), draft("drops", "scatter"))
	if code != http.StatusOK {
		t.Fatalf("preview answered %d: %s", code, body)
	}
	var view run.View
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatal(err)
	}
	if m := view.Blocks[0].Map; m == nil || len(m.Markers) != 3 {
		t.Fatalf("the draft's map drew %+v, want its three drops", view.Blocks[0])
	}
}

// Refused as publishing would refuse it, with the sentence that says what to
// fix: the author is who a sentence about the draft is for.
func TestADraftIsRefusedWherePublishingWouldBe(t *testing.T) {
	h, s := previewServer(t)
	editor := portal(t, s, "acme", "editor")
	for name, c := range map[string]struct{ report, says string }{
		"no such dataset":   {draft("nowhere", "scatter"), "nowhere"},
		"a layer it lacks":  {draft("drops", "polygon"), "geometry"},
		"not a report":      {"kind: Nonsense", "cannot decode"},
		"a measure average": {strings.Replace(draft("drops", "hexbin"), "aggregate: sum", "aggregate: avg", 1), "average"},
	} {
		code, body := preview(t, h, editor, c.report)
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, c.says) {
			t.Errorf("%s: %d %s, want 422 mentioning %q", name, code, body, c.says)
		}
	}
}

// Drawing a draft is publishing's reach, so it is publishing's bar: never a
// reader's, whose reports are the ones somebody granted them, nor an embedded
// page's.
func TestOnlySomebodyWhoMayPublishPreviews(t *testing.T) {
	h, s := previewServer(t)
	embed, err := s.Mint(token.Claims{Audience: token.Embed, Org: "acme", Project: "maps", Subject: "c-1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for who, c := range map[string]struct {
		tok  string
		want int
	}{
		"a reader":         {portal(t, s, "acme", "viewer"), http.StatusForbidden},
		"an embedded page": {embed, http.StatusUnauthorized},
		"nobody":           {"", http.StatusUnauthorized},
	} {
		if code, body := preview(t, h, c.tok, draft("drops", "scatter")); code != c.want {
			t.Errorf("%s: %d %s, want %d", who, code, body, c.want)
		}
	}
}

// An editor of another organisation's project is refused before the draft is
// looked at: checking it first answered "reads dataset nowhere" for a name
// that is not here and drew the map for one that is — an oracle for this
// project's datasets, one draft at a time.
func TestADraftFromAnotherProjectLearnsNothingHere(t *testing.T) {
	h, s := previewServer(t)
	rival := portal(t, s, "rival", "editor")
	for _, dataset := range []string{"drops", "nowhere"} {
		code, body := preview(t, h, rival, draft(dataset, "scatter"))
		if code != http.StatusForbidden || strings.Contains(body, "dataset") || strings.Contains(body, "Drops") {
			t.Errorf("a draft over %q from another project: %d %s, want the same 403 for both", dataset, code, body)
		}
	}
}
