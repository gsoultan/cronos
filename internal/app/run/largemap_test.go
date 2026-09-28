package run_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
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

// A map says which of the report's filters it can set: the one bound to the
// field it labels by, which a click on a region sets, and the area bound to
// its own coordinates, which its view sets — each with what it holds now.
func TestAMapSaysWhichFiltersItSets(t *testing.T) {
	s, _ := geoReport(t, dropDots)
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: picks}
spec:
  dataset: depots
  filters:
    - {name: region, type: enum, values: [England, Scotland], bind: {depots: region}}
    - {name: where, type: area, bind: {depots: "lat,lon"}}
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: chart
          chart: map
          title: Regions
          x: {field: region}
          y: {field: parcels, aggregate: sum}
          map: {layers: [polygon, scatter], geometry: shape, lat: lat, lon: lon}
        - kind: chart
          chart: map
          title: By staff
          x: {field: staff}
          y: {field: parcels, aggregate: sum}
          map: {layers: [scatter], lat: to_lat, lon: to_lon}`))
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Render(context.Background(), rep, run.Request{Filters: map[string]query.FilterValue{
		"region": {Op: query.In, Values: []any{"England"}},
		"where":  {Op: query.Within, Values: []any{50.0, -5.0, 56.0, 1.0}},
	}}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	m := v.Blocks[0].Map
	if m.Pick == nil || m.Pick.Filter != "region" || len(m.Pick.Values) != 1 || m.Pick.Values[0] != "England" {
		t.Errorf("pick = %+v", m.Pick)
	}
	if m.Area == nil || m.Area.Filter != "where" || m.Area.Op != "within" || len(m.Area.Values) != 4 {
		t.Errorf("area = %+v", m.Area)
	}
	// Labelled by a number, and placed by other coordinates: nothing to set.
	other := v.Blocks[1].Map
	if other.Pick != nil || other.Area != nil {
		t.Errorf("a map labelled by a number, at other coordinates, offers %+v and %+v", other.Pick, other.Area)
	}
}

const pickedRegions = `
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: picked}
spec:
  dataset: depots
  filters:
    - {name: region, type: enum, values: [England, Scotland], bind: {depots: region}}
  outputs:
    - name: interactive
      renderer: interactive
      layout: &layout
        - kind: chart
          chart: map
          title: Regions
          x: {field: region}
          y: {field: parcels, aggregate: sum}
          map: {layers: [scatter], lat: lat, lon: lon}
        - {kind: stat, label: Parcels, value: {field: parcels, aggregate: sum}}
    - name: pdf
      renderer: paginated
      layout: *layout
`

// labelsOn is the places a map drew, by name.
func labelsOn(m *run.GeoMap) map[string]bool {
	out := map[string]bool{}
	for _, p := range m.Markers {
		out[p.Label] = true
	}
	return out
}

/*
A map a click sets a filter from is that filter's control. It keeps every place
the filter can be set to, the picked one marked — narrowed by its own pick it
would show one place and no way to choose another — while the rest of the
report is narrowed by it, and so is the same map on paper, where nothing can
be clicked.
*/
func TestAMapThatPicksKeepsEveryPlaceItCanPick(t *testing.T) {
	s, _ := geoReport(t, dropDots)
	rep, err := yamlcodec.Loader{}.Report([]byte(pickedRegions))
	if err != nil {
		t.Fatal(err)
	}
	all, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	england := map[string]query.FilterValue{"region": {Op: query.In, Values: []any{"England"}}}
	v, err := s.Render(context.Background(), rep, run.Request{Filters: england}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	m := v.Blocks[0].Map
	if got := labelsOn(m); !got["England"] || !got["Scotland"] {
		t.Errorf("the map that picks drew %v, want every region", got)
	}
	if m.Pick == nil || len(m.Pick.Values) != 1 || m.Pick.Values[0] != "England" {
		t.Errorf("pick = %+v", m.Pick)
	}
	if v.Blocks[1].Value == all.Blocks[1].Value {
		t.Errorf("the stat beside it was not narrowed: %s either way", v.Blocks[1].Value)
	}
	paper, err := s.Render(context.Background(), rep, run.Request{Output: "pdf", Filters: england}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	if got := labelsOn(paper.Blocks[0].Map); got["Scotland"] || !got["England"] {
		t.Errorf("on paper the map drew %v, want England alone", got)
	}
}

// A large map that picks is asked for its views the same way: a view narrowed
// by the pick would empty the map around the one place picked the moment the
// reader zoomed.
func TestALargeMapThatPicksIsNotNarrowedByItInAnyView(t *testing.T) {
	s := depotsWith(t, grid(40))
	_, rep := geoReport(t, dropDots)
	rep.Filters = []definition.Filter{{Name: "drop", Type: definition.String,
		Bind: map[string]string{"depots": "depot"}}}
	one := map[string]query.FilterValue{"drop": {Op: query.In, Values: []any{"d5"}}}
	v, err := s.Render(context.Background(), rep, run.Request{Filters: one}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	m := v.Blocks[0].Map
	if m.Cells == nil || m.Pick == nil || m.Places != query.ChartLimit+40 {
		t.Fatalf("cells %v, pick %+v, %d places", m.Cells != nil, m.Pick, m.Places)
	}
	view, err := s.MapView(context.Background(), rep, run.ViewRequest{
		Request: run.Request{Filters: one}, View: run.Bounds{MaxX: 1, MaxY: 1}, Width: 800, Height: 600,
	}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range view.Cells.N {
		n += c
	}
	if n != query.ChartLimit+40 {
		t.Errorf("a view of the map that picks holds %d places, want all %d", n, query.ChartLimit+40)
	}
}

// twoDatasets is the depots table under two names — the depots, and "drops"
// over the same rows — for a map that draws one over the other.
func twoDatasets(t *testing.T, rows string) *run.Service {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:overlay%d?mode=memory&cache=shared", charts.Add(1)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	create := geoSchema[:strings.Index(geoSchema, "INSERT")]
	if _, err := db.Exec(create + rows); err != nil {
		t.Fatal(err)
	}
	depots, err := yamlcodec.Loader{}.Dataset([]byte(geoDataset))
	if err != nil {
		t.Fatal(err)
	}
	drops := depots
	drops.Name = "drops"
	return run.New(datasets{"depots": depots, "drops": drops}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
	}})
}

const overlaid = `- kind: chart
  chart: map
  title: Regions and drops
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map:
    layers: [scatter]
    lat: lat
    lon: lon
    overlays:
      - dataset: drops
        title: Every drop
        x: {field: depot}
        y: {field: parcels, aggregate: sum}
        map: {layers: [scatter], lat: to_lat, lon: to_lon}`

// Another dataset over the map: drawn, titled, and inside the box the map opens on.
func TestAMapDrawsAnotherDatasetOverItself(t *testing.T) {
	s := twoDatasets(t, geoSchema[strings.Index(geoSchema, "INSERT"):])
	_, rep := geoReport(t, overlaid)
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	m := v.Blocks[0].Map
	if len(m.Overlays) != 1 || m.Overlays[0].Title != "Every drop" || len(m.Overlays[0].Markers) == 0 {
		t.Fatalf("overlays = %+v", m.Overlays)
	}
	for _, p := range m.Overlays[0].Markers {
		if p.X < m.Bounds.MinX || p.X > m.Bounds.MaxX || p.Y < m.Bounds.MinY || p.Y > m.Bounds.MaxY {
			t.Errorf("%s is outside the box the map opens on", p.Label)
		}
	}
}

// An overlay with more places than fit is gathered like any map, and a view
// of it names it.
func TestALargeOverlayIsAskedForByItsOwnName(t *testing.T) {
	s := twoDatasets(t, grid(20))
	_, rep := geoReport(t, strings.Replace(overlaid, "lat: to_lat, lon: to_lon", "lat: lat, lon: lon", 1))
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	ov := v.Blocks[0].Map.Overlays[0]
	if ov.Cells == nil || ov.Detail == nil || ov.Detail.Overlay != 1 {
		t.Fatalf("a large overlay: cells %v, detail %+v", ov.Cells != nil, ov.Detail)
	}
	view, err := s.MapView(context.Background(), rep, run.ViewRequest{
		Block: 0, Overlay: 1, View: ov.Bounds, Width: 800, Height: 600,
	}, anyone())
	if err != nil || view.Cells == nil {
		t.Fatalf("a view of the overlay: %v", err)
	}
	if _, err := s.MapView(context.Background(), rep, run.ViewRequest{
		Block: 0, Overlay: 2, View: ov.Bounds, Width: 800, Height: 600,
	}, anyone()); !errors.Is(err, run.ErrNotAMap) {
		t.Errorf("an overlay that is not there: %v", err)
	}
}
