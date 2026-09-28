package run_test

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
	"github.com/gsoultan/cronos/internal/platform/h3"
)

/*
An h3 layer, against the port's own answers: every cell a place falls in, or
a warehouse indexed a row by, drawn once, holding exactly the total of what is
in it — on a map that fits, and on one that does not.
*/

// spot is a place on a grid around Amsterdam, a few hundred metres apart.
func spot(i int) (lat, lon float64) {
	return 52.35 + float64(i%61)*0.0021, 4.85 + float64(i/61)*0.0034
}

// h3Service is a table of places, each with its res-9 cell as text and as a
// number, and a map over it.
func h3Service(t *testing.T, n int, block string, extra ...string) (*run.Service, definition.Report) {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:h3cells%d?mode=memory&cache=shared", charts.Add(1)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var b strings.Builder
	b.WriteString("CREATE TABLE places (id TEXT, cell TEXT, num INTEGER, lat REAL, lon REAL, parcels REAL);\n")
	for i := 0; i < n; i++ {
		lat, lon := spot(i)
		c := h3.Cell(lat, lon, 9)
		fmt.Fprintf(&b, "INSERT INTO places VALUES ('p%d', '%s', %d, %.7f, %.7f, %d);\n", i, h3.String(c), int64(c), lat, lon, 1+i%5)
	}
	b.WriteString(strings.Join(extra, "\n"))
	if _, err := db.Exec(b.String()); err != nil {
		t.Fatal(err)
	}
	ds, err := yamlcodec.Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: places}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, cell, num, lat, lon, parcels FROM places
  fields:
    - {name: id,      type: string,  role: dimension}
    - {name: cell,    type: string,  role: dimension}
    - {name: num,     type: number,  role: dimension}
    - {name: lat,     type: decimal, role: dimension}
    - {name: lon,     type: decimal, role: dimension}
    - {name: parcels, type: decimal, role: measure, aggregate: sum}`))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: cells}
spec:
  dataset: places
  outputs:
    - name: interactive
      renderer: interactive
      layout:
` + indent(block)))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	return run.New(datasets{"places": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
	}}), rep
}

// binnedAt is the total in each cell places fall in at res, and rolledUp the
// total in each parent at res of the res-9 cells they were indexed by — not
// the same cells: H3's children only approximate their parent.
func binnedAt(n, res int) map[uint64]float64 {
	out := map[uint64]float64{}
	for i := 0; i < n; i++ {
		lat, lon := spot(i)
		out[h3.Cell(lat, lon, res)] += float64(1 + i%5)
	}
	return out
}

func rolledUp(n, res int) map[uint64]float64 {
	out := map[uint64]float64{}
	for i := 0; i < n; i++ {
		lat, lon := spot(i)
		out[h3.Parent(h3.Cell(lat, lon, 9), res)] += float64(1 + i%5)
	}
	return out
}

func drawnMap(t *testing.T, s *run.Service, rep definition.Report) *run.GeoMap {
	t.Helper()
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	return v.Blocks[0].Map
}

// sameTotals reports whether the cells drawn hold the totals expected: the
// same number of cells, and the same totals in them.
func sameTotals(t *testing.T, got []run.Shape, want map[uint64]float64) {
	t.Helper()
	var g, w []float64
	for _, s := range got {
		g = append(g, s.Value)
	}
	for _, v := range want {
		w = append(w, v)
	}
	slices.Sort(g)
	slices.Sort(w)
	if !slices.Equal(g, w) {
		t.Errorf("%d cells drawn, %d expected; totals differ", len(g), len(w))
		labels := map[string]bool{}
		for _, s := range got {
			labels[s.Label] = true
		}
		for c, v := range want {
			if !labels[h3.String(c)] {
				t.Logf("not drawn: %s holding %v", h3.String(c), v)
			}
		}
	}
}

func TestPlacesAreBinnedIntoTheCellsTheyAreIn(t *testing.T) {
	s, rep := h3Service(t, 400, `- kind: chart
  chart: map
  title: Parcels per cell
  x: {field: id}
  y: {field: parcels, aggregate: sum}
  map: {layers: [h3], lat: lat, lon: lon, h3Resolution: 7}`)
	m := drawnMap(t, s, rep)
	sameTotals(t, m.Hexes, binnedAt(400, 7))
	// A cell of places says how many it holds, as a hexagon does.
	if len(m.Hexes) == 0 || !strings.Contains(m.Hexes[0].Label, "location") || m.Hexes[0].Path == "" {
		t.Fatalf("cells %+v", m.Hexes)
	}
}

// Cells a warehouse indexed, as text or as the number itself, drawn as they
// are — and added up to a coarser resolution by parent, as H3 rolls up.
func TestIndexedCellsAreDrawnAndRolledUp(t *testing.T) {
	for _, field := range []string{"cell", "num"} {
		s, rep := h3Service(t, 400, `- kind: chart
  chart: map
  title: Parcels per cell
  x: {field: `+field+`}
  y: {field: parcels, aggregate: sum}
  map: {layers: [h3], h3: `+field+`}`)
		m := drawnMap(t, s, rep)
		sameTotals(t, m.Hexes, rolledUp(400, 9))
		if c, ok := h3.Parse(m.Hexes[0].Label); !ok || h3.Resolution(c) != 9 {
			t.Errorf("%s: a cell is labelled %q, want its id", field, m.Hexes[0].Label)
		}
		rep.Outputs[0].Layout[0].Map.H3Resolution = 6
		sameTotals(t, drawnMap(t, s, rep).Hexes, rolledUp(400, 6))
	}
}

// A value that is not a cell is left out, as a place with no coordinates is:
// a column holds somebody's data, and a wrong id drawn somewhere is worse
// than a row not drawn at all.
func TestAValueThatIsNotACellIsLeftOut(t *testing.T) {
	s, rep := h3Service(t, 50, `- kind: chart
  chart: map
  title: Parcels per cell
  x: {field: cell}
  y: {field: parcels, aggregate: sum}
  map: {layers: [h3], h3: cell}`,
		"INSERT INTO places VALUES ('x1', 'nonsense', 0, 0, 0, 100);",
		"INSERT INTO places VALUES ('x2', '8f28308280f18f2f', 0, 0, 0, 100);",
		"INSERT INTO places VALUES ('x3', NULL, NULL, 0, 0, 100);")
	sameTotals(t, drawnMap(t, s, rep).Hexes, rolledUp(50, 9))
}

// A map of more places than a map holds bins every one of them, and one of
// more indexed cells than that draws every cell.
func TestALargeH3MapCountsEveryRow(t *testing.T) {
	n := query.ChartLimit + 300
	s, rep := h3Service(t, n, `- kind: chart
  chart: map
  title: Parcels per cell
  x: {field: id}
  y: {field: parcels, aggregate: sum}
  map: {layers: [h3], lat: lat, lon: lon, h3Resolution: 8}`)
	m := drawnMap(t, s, rep)
	sameTotals(t, m.Hexes, binnedAt(n, 8))
	if m.Places != n {
		t.Errorf("%d places counted, want %d", m.Places, n)
	}

	cells, crep := h3Service(t, n, `- kind: chart
  chart: map
  title: Parcels per cell
  x: {field: id}
  y: {field: parcels, aggregate: sum}
  map: {layers: [h3], h3: cell}`)
	sameTotals(t, drawnMap(t, cells, crep).Hexes, rolledUp(n, 9))
}
