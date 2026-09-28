package run_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/query"
)

const dropDots = `- kind: chart
  chart: map
  title: Drops
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  map: {layers: [scatter], lat: lat, lon: lon}`

// gridAt is where grid puts drop i, as the world units a viewer draws in.
func gridAt(i int) (x, y float64) {
	return world(51.5+float64(i%97)*0.0003, -0.12+float64(i/97)*0.0005)
}

func world(lat, lon float64) (x, y float64) {
	rad := lat * math.Pi / 180
	return (lon + 180) / 360, 0.5 - math.Log(math.Tan(rad)+1/math.Cos(rad))/(2*math.Pi)
}

// A reader zooms into part of a large map. The view holds the places in it —
// every one of them, one per cell at this depth, with its own label.
func TestAViewOfALargeMapIsThePlacesInIt(t *testing.T) {
	s, rep := geoReport(t, dropDots)
	s = depotsWith(t, grid(1))

	// Rows 30 to 39 of columns 10 to 13 — edges halfway between drops, so no
	// drop is on one and the answer does not hang on the fifteenth decimal.
	x0, y0 := world(51.5+29.5*0.0003, -0.12+9.5*0.0005)
	x1, y1 := world(51.5+39.5*0.0003, -0.12+13.5*0.0005)
	view := run.Bounds{MinX: min(x0, x1), MinY: min(y0, y1), MaxX: max(x0, x1), MaxY: max(y0, y1)}
	m, err := s.MapView(context.Background(), rep, run.ViewRequest{
		Block: 0, View: view, Width: 1600, Height: 1600,
	}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	for i := 1; i <= query.ChartLimit+1; i++ {
		x, y := gridAt(i)
		if x >= view.MinX && x <= view.MaxX && y >= view.MinY && y <= view.MaxY {
			want++
		}
	}
	if m.Cells == nil {
		t.Fatal("a view came back with no cells")
	}
	inside, lone := 0, 0
	for i, n := range m.Cells.N {
		x, y := m.Cells.X[i], m.Cells.Y[i]
		if x >= view.MinX && x <= view.MaxX && y >= view.MinY && y <= view.MaxY {
			inside += n
		}
		if n == 1 && strings.HasPrefix(m.Cells.L[i], "d") {
			lone++
		}
	}
	if inside != want {
		t.Errorf("the view holds %d places, want %d", inside, want)
	}
	if lone != len(m.Cells.N) {
		t.Errorf("%d of %d cells are a single named place at this depth", lone, len(m.Cells.N))
	}
}

// A view is a render of less of the world: the same rules, applied again.
func TestAViewIsRefusedWhatARenderWouldBe(t *testing.T) {
	s, rep := geoReport(t, dropDots)
	ok := run.ViewRequest{Block: 0, View: run.Bounds{MaxX: 1, MaxY: 1}, Width: 800, Height: 600}
	for name, c := range map[string]func(r run.ViewRequest) run.ViewRequest{
		"a block that is not there": func(r run.ViewRequest) run.ViewRequest { r.Block = 3; return r },
		"a view that is not a number": func(r run.ViewRequest) run.ViewRequest {
			r.View.MaxX = math.NaN()
			return r
		},
		"a view with no area":           func(r run.ViewRequest) run.ViewRequest { r.View.MaxX = 0; return r },
		"a stage a million pixels wide": func(r run.ViewRequest) run.ViewRequest { r.Width = 1_000_000; return r },
	} {
		if _, err := s.MapView(context.Background(), rep, c(ok), anyone()); !errors.Is(err, run.ErrNotAMap) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := s.MapView(context.Background(), rep, ok, anyone()); err != nil {
		t.Errorf("a whole-world view of a small map: %v", err)
	}
}

// Each view is its own query, so a category's colour cannot come from the
// order the view's rows arrive in. It comes from the map as it opened.
func TestACategoryKeepsItsColourInEveryView(t *testing.T) {
	block := strings.Replace(dropDots, "  map:", "  series: {field: carrier}\n  map:", 1)
	rows := strings.Replace(grid(40), "'Aurora'", "CASE WHEN i % 3 = 0 THEN 'Baltic' ELSE 'Aurora' END", 1)
	s := depotsWith(t, rows)
	_, rep := geoReport(t, block)

	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	m := v.Blocks[0].Map
	if m.Detail == nil || len(m.Detail.Categories) != 2 || m.Detail.Categories[0] != "Aurora" {
		t.Fatalf("the categories, most common first: %+v", m.Detail)
	}
	// Deep enough that every cell is one drop, so each can be checked against
	// the carrier it is: Aurora the first swatch, Baltic the second.
	x0, y0 := gridAt(0)
	x1, y1 := gridAt(96)
	view, err := s.MapView(context.Background(), rep, run.ViewRequest{
		Block: 0, View: run.Bounds{MinX: x0 - 1e-6, MinY: min(y0, y1), MaxX: x1 + 1e-6, MaxY: max(y0, y1)},
		Width: 1600, Height: 1600, Categories: m.Detail.Categories,
	}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for i, label := range view.Cells.L {
		var d int
		if _, err := fmt.Sscanf(label, "d%d", &d); err != nil {
			continue
		}
		want := 0
		if d%3 == 0 {
			want = 1
		}
		if view.Cells.S[i] != want {
			t.Errorf("drop %d is in swatch %d, want %d", d, view.Cells.S[i], want)
		}
		checked++
	}
	if checked < 50 {
		t.Errorf("only %d drops came back as themselves", checked)
	}
}

// Routes between more places than the cap: gathered between cells, the
// busiest kept first, every flow counted into the one it belongs to.
func TestFlowsPastTheCapAreGatheredIntoRoutes(t *testing.T) {
	block := `- kind: chart
  chart: map
  title: Transfers
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  map: {layers: [flow], lat: lat, lon: lon, toLat: to_lat, toLon: to_lon}`
	rows := strings.Replace(grid(10), "0, 0,\n       'Aurora'", "55.95, -3.19,\n       'Aurora'", 1)
	v := renderOne(t, depotsWith(t, rows), block)

	if v.Map.Detail == nil || len(v.Map.Arcs) == 0 {
		t.Fatalf("a flow map past the cap drew %d arcs", len(v.Map.Arcs))
	}
	total := 0.0
	for _, a := range v.Map.Arcs {
		total += a.Value
	}
	if total != query.ChartLimit+10 {
		t.Errorf("the routes carry %v parcels, want all %d", total, query.ChartLimit+10)
	}
}

// Regions shaded from the places under them, past the cap: each region's
// total is over its rows, not over the first five thousand.
func TestRegionsPastTheCapAreTotalledOverEveryRow(t *testing.T) {
	block := `- kind: chart
  chart: map
  title: Drops by region
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map: {layers: [polygon, scatter], geometry: shape, lat: lat, lon: lon}`
	rows := strings.Replace(grid(25), "'England', ''",
		`'England', '{"type":"Polygon","coordinates":[[[-0.5,51.2],[0.3,51.2],[0.3,51.7],[-0.5,51.7],[-0.5,51.2]]]}'`, 1)
	v := renderOne(t, depotsWith(t, rows), block)

	if len(v.Map.Shapes) != 1 || v.Map.Shapes[0].Value != query.ChartLimit+25 {
		t.Fatalf("England holds %+v, want one region of %d", v.Map.Shapes, query.ChartLimit+25)
	}
	if v.Map.Cells == nil {
		t.Error("the places over the region were not drawn")
	}
}

// Cells are written by hand to keep them small. They must still be JSON a
// browser parses, and say what the struct says.
func TestCellsAreJSON(t *testing.T) {
	v := renderOne(t, depotsWith(t, grid(1)), dropDots)
	raw, err := json.Marshal(v.Map)
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Cells struct {
			Size float64   `json:"size"`
			X    []float64 `json:"x"`
			N    []int     `json:"n"`
			L    []string  `json:"l"`
		} `json:"cells"`
	}
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("not JSON: %v\n%.300s", err, raw)
	}
	if len(back.Cells.X) != len(v.Map.Cells.X) || len(back.Cells.L) != len(back.Cells.N) || back.Cells.Size <= 0 {
		t.Errorf("round trip: %d x, %d n, %d l, size %v", len(back.Cells.X), len(back.Cells.N),
			len(back.Cells.L), back.Cells.Size)
	}
	if per := float64(len(raw)) / float64(len(v.Map.Cells.X)); per > 60 {
		t.Errorf("%.0f bytes a cell — the point of writing them by hand is to be small", per)
	}
}
