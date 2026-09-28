package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
	"github.com/gsoultan/cronos/internal/platform/token"

	_ "modernc.org/sqlite"
)

// oneReport answers for the one report a test defines.
type oneReport struct{ r definition.Report }

func (o oneReport) Report(_ context.Context, name string) (definition.Report, error) {
	if name != o.r.Name {
		return definition.Report{}, errors.New("no such report")
	}
	return o.r, nil
}

type fixedDatasets map[string]definition.Dataset

func (f fixedDatasets) Dataset(_ context.Context, name string) (definition.Dataset, error) {
	ds, ok := f[name]
	if !ok {
		return definition.Dataset{}, errors.New("no such dataset")
	}
	return ds, nil
}

/*
A map of two customers' drops, more of each than a map holds, behind row-level
security: each customer's embedded reader sees their own drops and none of the
other's. The rule a render has always kept, kept by the view too — a view is a
render of less of the world, and zoomed in it is the places themselves.
*/
func mapServer(t *testing.T) (http.Handler, *token.Signer) {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:mapviews%d?mode=memory&cache=shared", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(fmt.Sprintf(`
CREATE TABLE drops (id TEXT, customer TEXT, lat REAL, lon REAL, parcels REAL);
WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < %d)
INSERT INTO drops SELECT 'd' || i, CASE WHEN i %% 2 = 0 THEN 'acme' ELSE 'globex' END,
  51.5 + (i %% 97) * 0.0003, -0.12 + (i / 97) * 0.0005, 1 FROM n;`, 2*query.ChartLimit+300)); err != nil {
		t.Fatal(err)
	}
	ds, err := yamlcodec.Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: drops}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, customer, lat, lon, parcels FROM drops
  fields:
    - {name: id,       type: string,  role: dimension}
    - {name: customer, type: string,  role: dimension}
    - {name: lat,      type: decimal, role: dimension}
    - {name: lon,      type: decimal, role: dimension}
    - {name: parcels,  type: decimal, role: measure, aggregate: sum}
  rowLevelSecurity:
    - predicate: customer = {{ .scope.customer }}`))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: drops}
spec:
  dataset: drops
  outputs:
    - name: screen
      renderer: interactive
      layout:
        - kind: chart
          chart: map
          title: Drops
          x: {field: id}
          y: {field: parcels, aggregate: sum}
          map: {layers: [scatter], lat: lat, lon: lon}`))
	if err != nil {
		t.Fatal(err)
	}
	runner := run.New(fixedDatasets{"drops": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
	}})
	signer, err := token.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	one := &api.One{Org: "acme-isv", ProjectID: "maps",
		Only: &api.Project{Reports: oneReport{rep}, Runner: runner}}
	return api.Routes(api.Deps{Projects: one, Signer: signer, Log: logger(&bytes.Buffer{})}), signer
}

func embedToken(t *testing.T, s *token.Signer, customer, report string) string {
	t.Helper()
	tok, err := s.Mint(token.Claims{Audience: token.Embed, Org: "acme-isv", Project: "maps",
		Subject: customer + "-reader", Scope: map[string]string{"customer": customer}, Report: report}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func postJSON(h http.Handler, path, tok string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAMapViewShowsAReaderOnlyTheirOwnRows(t *testing.T) {
	h, s := mapServer(t)
	acme, globex := embedToken(t, s, "acme", ""), embedToken(t, s, "globex", "")

	// The whole map as it opens, for acme: large, and only acme's.
	w := postJSON(h, "/v1/embed/reports/drops", acme, map[string]any{})
	var opened struct {
		Blocks []struct{ Map run.GeoMap } `json:"blocks"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &opened) != nil {
		t.Fatalf("render answered %d: %.200s", w.Code, w.Body)
	}
	m := opened.Blocks[0].Map
	if m.Places != query.ChartLimit+150 || m.Detail == nil {
		t.Fatalf("acme's map holds %d places, want %d, and a way to ask for more: %+v",
			m.Places, query.ChartLimit+150, m.Detail)
	}

	// Zoomed into one column of drops, where every cell is one drop and says
	// whose it is by its label: even ids are acme's, odd ones globex's.
	x0, y0 := worldOf(51.5305, -0.1201)
	x1, y1 := worldOf(51.4995, -0.1199)
	view := map[string]any{"block": 0, "width": 1600, "height": 1600,
		"view": []float64{x0, y0, x1, y1}}
	for who, tok := range map[string]string{"acme": acme, "globex": globex} {
		w := postJSON(h, "/v1/embed/reports/drops/map", tok, view)
		if w.Code != http.StatusOK {
			t.Fatalf("%s's view answered %d: %s", who, w.Code, w.Body)
		}
		var got run.GeoMap
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Cells == nil {
			t.Fatalf("%s's view: %v %.200s", who, err, w.Body)
		}
		seen := 0
		for _, label := range got.Cells.L {
			var d int
			if _, err := fmt.Sscanf(label, "d%d", &d); err != nil {
				continue
			}
			seen++
			if (d%2 == 0) != (who == "acme") {
				t.Errorf("%s's view holds drop %d, which is the other customer's", who, d)
			}
		}
		// The first column holds 96 drops, half of them each customer's.
		if seen < 40 {
			t.Errorf("%s's view holds %d of their drops — a view of nothing proves nothing", who, seen)
		}
	}
}

// worldOf is a coordinate in the world units a viewer sends.
func worldOf(lat, lon float64) (x, y float64) {
	rad := lat * math.Pi / 180
	return (lon + 180) / 360, 0.5 - math.Log(math.Tan(rad)+1/math.Cos(rad))/(2*math.Pi)
}

// The checks a render makes are the checks a view makes, from the same code.
func TestAMapViewRefusesWhatARenderRefuses(t *testing.T) {
	h, s := mapServer(t)
	view := map[string]any{"block": 0, "width": 800, "height": 600, "view": []float64{0, 0, 1, 1}}

	if w := postJSON(h, "/v1/embed/reports/drops/map", "not-a-token", view); w.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", w.Code)
	}
	pinned := embedToken(t, s, "acme", "another-report")
	if w := postJSON(h, "/v1/embed/reports/drops/map", pinned, view); w.Code != http.StatusForbidden {
		t.Errorf("a token pinned to another report: %d", w.Code)
	}
	tok := embedToken(t, s, "acme", "")
	bad := map[string]any{"block": 7, "width": 800, "height": 600, "view": []float64{0, 0, 1, 1}}
	if w := postJSON(h, "/v1/embed/reports/drops/map", tok, bad); w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), "not a view of a map") {
		t.Errorf("a block that is not there: %d %s", w.Code, w.Body)
	}
	unknown := map[string]any{"block": 0, "zoom": 3}
	if w := postJSON(h, "/v1/embed/reports/drops/map", tok, unknown); w.Code != http.StatusBadRequest {
		t.Errorf("a field the endpoint does not take: %d", w.Code)
	}
}
