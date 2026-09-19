package run_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
	_ "modernc.org/sqlite"
)

/*
 * The chart shapes, end to end against a real database.
 *
 * Same argument as service_test.go: the claims worth testing — that a grouped
 * chart comes back dense, that a choropleth's geometry survives projection,
 * that a map folds its regions with the right function — are claims about what
 * a database returns and what this package does with it. A fake executor would
 * assert that Go can build a struct.
 */

const geoSchema = `
CREATE TABLE depots (
  region TEXT, shape TEXT, lat REAL, lon REAL,
  to_lat REAL, to_lon REAL, carrier TEXT, parcels REAL, staff REAL
);
INSERT INTO depots VALUES
  ('England','{"type":"Polygon","coordinates":[[[-0.5,51.2],[0.3,51.2],[0.3,51.7],[-0.5,51.7],[-0.5,51.2]]]}',
   51.5074, -0.1278, 53.4808, -2.2426, 'Aurora',  1200, 40),
  ('England','{"type":"Polygon","coordinates":[[[-0.5,51.2],[0.3,51.2],[0.3,51.7],[-0.5,51.7],[-0.5,51.2]]]}',
   51.4545, -2.5879, 53.4808, -2.2426, 'Baltic',   300, 12),
  ('Scotland','{"type":"Polygon","coordinates":[[[-4.5,55.7],[-3.0,55.7],[-3.0,56.2],[-4.5,56.2],[-4.5,55.7]]]}',
   55.9533, -3.1883, 51.5074, -0.1278, 'Aurora',   700, 25);`

const geoDataset = `
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: depots}
spec:
  sources: [{ref: warehouse}]
  query: SELECT region, shape, lat, lon, to_lat, to_lon, carrier, parcels, staff FROM depots
  fields:
    - {name: region,  type: string,  role: dimension, label: Region}
    - {name: shape,   type: string,  role: dimension, hidden: true}
    - {name: lat,     type: decimal, role: dimension, hidden: true}
    - {name: lon,     type: decimal, role: dimension, hidden: true}
    - {name: to_lat,  type: decimal, role: dimension, hidden: true}
    - {name: to_lon,  type: decimal, role: dimension, hidden: true}
    - {name: carrier, type: string,  role: dimension, label: Carrier}
    - {name: parcels, type: decimal, role: measure, aggregate: sum, label: Parcels}
    - {name: staff,   type: decimal, role: measure, aggregate: sum, label: Staff}`

// charts names each test's own in-memory database.
var charts atomic.Int64

// geoReport renders one output whose single block is the YAML passed in, so
// each test states only the block it is about.
func geoReport(t *testing.T, block string) (*run.Service, definition.Report) {
	t.Helper()

	// A database per call. The DSN used to be one shared name, which meant a
	// test that set up twice re-ran the schema against a table already there.
	db, err := sql.Open("sqlite",
		fmt.Sprintf("file:charts%d?mode=memory&cache=shared", charts.Add(1)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(geoSchema); err != nil {
		t.Fatal(err)
	}

	ds, err := yamlcodec.Loader{}.Dataset([]byte(geoDataset))
	if err != nil {
		t.Fatalf("dataset: %v", err)
	}
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: depot-map}
spec:
  dataset: depots
  outputs:
    - name: interactive
      renderer: interactive
      layout:
` + indent(block)))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	return run.New(datasets{"depots": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db),
		Builder:  query.NewBuilder(query.SQLite{}),
	}}), rep
}

func indent(s string) string {
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		out.WriteString("        " + line + "\n")
	}
	return out.String()
}

func draw(t *testing.T, block string) run.Block {
	t.Helper()
	s, rep := geoReport(t, block)
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(v.Blocks) != 1 {
		t.Fatalf("want one block, got %d", len(v.Blocks))
	}
	return v.Blocks[0]
}

func anyone() principal.Principal {
	return principal.Principal{Subject: "u1", OrgID: "o1", ProjectID: "p1",
		ProjectRole: principal.ProjectViewer}
}

func TestAGroupedChartComesBackDense(t *testing.T) {
	// Baltic has no Scottish depot. A stacked chart that only carries the
	// buckets each series matched cannot line its segments up, so the reader
	// pads it here — once — rather than every renderer doing it separately.
	b := draw(t, `- kind: chart
  chart: bar
  title: Parcels by region
  stacked: true
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)

	if len(b.Groups) != 2 {
		t.Fatalf("want a group per carrier, got %d: %+v", len(b.Groups), b.Groups)
	}
	if !b.Stacked {
		t.Error("the block said stacked: true and the payload did not")
	}
	for _, g := range b.Groups {
		if len(g.Bars) != 2 {
			t.Errorf("%s covers %d buckets, want England and Scotland — a series "+
				"missing a bucket must still carry it", g.Label, len(g.Bars))
		}
	}
	if b.Groups[0].Slot == b.Groups[1].Slot {
		t.Error("two series share a colour slot")
	}
	// Baltic's Scotland cell is the one that had no row at all.
	for _, g := range b.Groups {
		if g.Label != "Baltic" {
			continue
		}
		for _, bar := range g.Bars {
			if bar.Label == "Scotland" && bar.Value != 0 {
				t.Errorf("Baltic/Scotland = %v, want a padded zero", bar.Value)
			}
		}
	}
}

func TestASingleSeriesChartIsUnchanged(t *testing.T) {
	// The shape every pinned copy of the embed bundle already reads.
	b := draw(t, `- kind: chart
  chart: bar
  title: Parcels by region
  x: {field: region}
  y: {field: parcels, aggregate: sum}`)

	if len(b.Series) != 2 || len(b.Groups) != 0 {
		t.Fatalf("a chart with no series dimension should still fill series: %+v", b)
	}
	if b.Series[0].Label != "England" || b.Series[0].Value != 1500 {
		t.Errorf("series[0] = %+v, want England at 1500", b.Series[0])
	}
}

func TestAChoroplethProjectsItsGeometry(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: map
  title: Parcels by region
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map: {geometry: shape}`)

	if b.Map == nil {
		t.Fatal("a map block rendered no map")
	}
	if len(b.Map.Shapes) != 2 {
		t.Fatalf("want a shape per region, got %d", len(b.Map.Shapes))
	}
	for _, s := range b.Map.Shapes {
		if !strings.HasPrefix(s.Path, "M") || !strings.HasSuffix(s.Path, "Z") {
			t.Errorf("%s path is not a closed subpath: %q", s.Label, s.Path)
		}
	}
	// England's two depots sum to 1500 against Scotland's 700, so England
	// must land in a higher band than Scotland or the shading says nothing.
	var england, scotland run.Shape
	for _, s := range b.Map.Shapes {
		switch s.Label {
		case "England":
			england = s
		case "Scotland":
			scotland = s
		}
	}
	if england.Value != 1500 || scotland.Value != 700 {
		t.Errorf("values = %v / %v, want 1500 / 700", england.Value, scotland.Value)
	}
	if england.Step <= scotland.Step {
		t.Errorf("England (%v) is not shaded above Scotland (%v)", england.Step, scotland.Step)
	}
	if len(b.Map.Legend) == 0 {
		t.Error("a shaded map with no legend is a set of colours nobody can read")
	}
	if b.Map.Bounds.MaxX <= b.Map.Bounds.MinX {
		t.Errorf("bounds are collapsed: %+v", b.Map.Bounds)
	}
}

func TestAMapCombinesPolygonsWithPoints(t *testing.T) {
	// The combination the single-plan design has to earn: one query at point
	// grain, with the regions folded up from it.
	b := draw(t, `- kind: chart
  chart: map
  title: Depots
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map:
    layers: [polygon, scatter]
    geometry: shape
    lat: lat
    lon: lon`)

	if len(b.Map.Shapes) != 2 {
		t.Fatalf("want two regions, got %d", len(b.Map.Shapes))
	}
	if len(b.Map.Markers) != 3 {
		t.Fatalf("want a marker per depot, got %d", len(b.Map.Markers))
	}
	for _, s := range b.Map.Shapes {
		if s.Label == "England" && s.Value != 1500 {
			t.Errorf("England = %v, want its two depots folded to 1500", s.Value)
		}
	}
	if got := strings.Join(b.Map.Layers, ","); got != "polygon,scatter" {
		t.Errorf("layers = %q, want them in draw order", got)
	}
}

func TestAMapThatWouldAverageAveragesIsRefused(t *testing.T) {
	// Running at point grain and folding regions from it is exact for sum,
	// count, min and max. It is not for avg, and an average of averages is a
	// number nobody measured that looks entirely plausible.
	//
	// Refused when the report is loaded rather than when it is rendered, which
	// is the earlier of the two places it can be caught: the author who typed
	// `avg` is still the one holding it.
	err := load(`- kind: chart
  chart: map
  title: Staff
  x: {field: region}
  y: {field: staff, aggregate: avg}
  map:
    layers: [polygon, bubble]
    geometry: shape
    lat: lat
    lon: lon`)
	if err == nil {
		t.Fatal("an averaged choropleth over points stored without complaint")
	}
	if !strings.Contains(err.Error(), "average") {
		t.Errorf("the error does not say what is wrong: %v", err)
	}
}

// load parses a report carrying one block, and returns why it could not be
// stored.
func load(block string) error {
	_, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: depot-map}
spec:
  dataset: depots
  outputs:
    - name: interactive
      renderer: interactive
      layout:
` + indent(block)))
	return err
}

func TestFlowsCarryBothEnds(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: map
  title: Transfers
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map:
    layers: [flow]
    lat: lat
    lon: lon
    toLat: to_lat
    toLon: to_lon`)

	if len(b.Map.Arcs) != 3 {
		t.Fatalf("want an arc per depot, got %d", len(b.Map.Arcs))
	}
	for _, a := range b.Map.Arcs {
		if a.X1 == a.X2 && a.Y1 == a.Y2 {
			t.Errorf("%s is an arc that goes nowhere: %+v", a.Label, a)
		}
		if a.Weight < 0 || a.Weight > 1 {
			t.Errorf("%s has weight %v, which is not a fraction", a.Label, a.Weight)
		}
	}
}

func TestABubbleSizesByItsOwnMeasure(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: bubble
  title: Parcels against staff
  x: {field: region}
  xValue: {field: staff, aggregate: sum}
  y: {field: parcels, aggregate: sum}
  size: {field: staff, aggregate: sum}`)

	if len(b.Points) != 2 {
		t.Fatalf("want a dot per region, got %d", len(b.Points))
	}
	if b.XAxis == nil || b.YAxis == nil || len(b.XAxis.Ticks) < 2 {
		t.Fatalf("a plot needs both scales: %+v %+v", b.XAxis, b.YAxis)
	}
	// England has 52 staff against Scotland's 25, so its bubble is larger.
	var england, scotland run.Point
	for _, p := range b.Points {
		switch p.Label {
		case "England":
			england = p
		case "Scotland":
			scotland = p
		}
	}
	if !(england.Weight > scotland.Weight) {
		t.Errorf("England (%v) is not drawn larger than Scotland (%v)",
			england.Weight, scotland.Weight)
	}
	if england.Size == "" {
		t.Error("a bubble carries its size measure formatted, for the tooltip")
	}
}

func TestEveryChartStillFillsSeries(t *testing.T) {
	// `series` is the chart kind's collection, so a viewer reading
	// `series.map` must not crash on a type that keeps its points elsewhere.
	for _, block := range []string{
		`- kind: chart
  chart: map
  title: m
  x: {field: region}
  y: {field: parcels}
  map: {geometry: shape}`,
		`- kind: chart
  chart: scatter
  title: s
  x: {field: region}
  xValue: {field: staff}
  y: {field: parcels}`,
		`- kind: chart
  chart: bar
  title: g
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels}`,
	} {
		b := draw(t, block)
		if b.Series == nil {
			t.Errorf("%s chart left series nil rather than empty", b.Chart)
		}
	}
}

// marshal renders one block the way the API does.
func marshal(t *testing.T, b run.Block) map[string]any {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestTheWireCarriesWhatEachChartTypeOwns(t *testing.T) {
	// The payload is the contract packages/embed/src/types.ts mirrors, and it
	// is assembled by hand in MarshalJSON rather than by struct tags — so a
	// field added to the struct and forgotten there is invisible until a chart
	// renders empty in somebody's browser.
	for _, c := range []struct {
		name  string
		block string
		want  []string
		gone  []string
	}{{
		name: "a single-series bar carries series and nothing else",
		block: `- kind: chart
  chart: bar
  title: t
  x: {field: region}
  y: {field: parcels}`,
		want: []string{"chart", "series"},
		gone: []string{"groups", "points", "map", "xAxis", "yAxis"},
	}, {
		name: "a stacked bar carries its groups and its totals",
		block: `- kind: chart
  chart: bar
  title: t
  stacked: true
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels}`,
		want: []string{"groups", "stacked", "totals", "series"},
		gone: []string{"points", "map"},
	}, {
		name: "a line carries the scale its points are read against",
		block: `- kind: chart
  chart: line
  title: t
  x: {field: region}
  y: {field: parcels}`,
		want: []string{"series", "yAxis"},
		gone: []string{"xAxis", "groups", "map"},
	}, {
		name: "a scatter carries both scales",
		block: `- kind: chart
  chart: scatter
  title: t
  x: {field: region}
  xValue: {field: staff}
  y: {field: parcels}`,
		want: []string{"points", "xAxis", "yAxis", "series"},
		gone: []string{"groups", "map"},
	}, {
		name: "a map carries its layers",
		block: `- kind: chart
  chart: map
  title: t
  x: {field: region}
  y: {field: parcels}
  map: {geometry: shape}`,
		want: []string{"map", "series"},
		gone: []string{"groups", "points", "xAxis"},
	}} {
		t.Run(c.name, func(t *testing.T) {
			out := marshal(t, draw(t, c.block))
			for _, k := range c.want {
				if _, ok := out[k]; !ok {
					t.Errorf("%q is missing from the payload: %v", k, keys(out))
				}
			}
			for _, k := range c.gone {
				if _, ok := out[k]; ok {
					t.Errorf("%q is in the payload of a chart that does not own it", k)
				}
			}
		})
	}
}

func TestAMapsListsAreEmptyRatherThanAbsent(t *testing.T) {
	// A client should never have to tell an absent list from an empty one, and
	// the emptiest report is the one most likely to be the first that arrives.
	b := draw(t, `- kind: chart
  chart: map
  title: t
  x: {field: region}
  y: {field: parcels}
  map: {layers: [scatter], lat: lat, lon: lon}`)

	raw, err := json.Marshal(b.Map)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"shapes", "markers", "arcs", "legend"} {
		if _, ok := m[k]; !ok {
			t.Errorf("%q is absent — a viewer reading %s.map crashes on it", k, k)
		}
	}
	// No basemap was asked for, so none is promised.
	if _, ok := m["tiles"]; ok {
		t.Error("a map nobody gave a basemap to is offering one")
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestAComboDrawsEachMeasureItsOwnWay(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: combo
  title: Parcels and staff
  x: {field: region}
  metrics:
    - {field: parcels, aggregate: sum, label: Parcels, draw: bar}
    - {field: staff, aggregate: sum, label: Staff, draw: line}`)

	if len(b.Tracks) != 2 {
		t.Fatalf("want a track per measure, got %d", len(b.Tracks))
	}
	if b.Tracks[0].Draw != "bar" || b.Tracks[1].Draw != "line" {
		t.Errorf("tracks drew %q and %q, want bar then line", b.Tracks[0].Draw, b.Tracks[1].Draw)
	}
	for _, tr := range b.Tracks {
		if len(tr.Bars) != 2 {
			t.Errorf("%s covers %d buckets, want England and Scotland", tr.Label, len(tr.Bars))
		}
	}
	// One shared axis unless a measure asked to leave it, which neither did.
	if b.Axis2 != nil {
		t.Error("a combo nobody asked for a second axis on grew one")
	}
	if b.YAxis == nil || len(b.YAxis.Ticks) < 2 {
		t.Fatalf("a combo needs a scale: %+v", b.YAxis)
	}
}

func TestASecondAxisIsOptedIntoPerMeasure(t *testing.T) {
	// Two scales on one plot is the most-flagged mistake in charting, so it is
	// reachable and never a default — this is the test that it is reachable.
	b := draw(t, `- kind: chart
  chart: combo
  title: Parcels against staff
  x: {field: region}
  metrics:
    - {field: parcels, aggregate: sum, draw: bar}
    - {field: staff, aggregate: sum, draw: line, axis: secondary}`)

	if b.Axis2 == nil {
		t.Fatal("a measure asked for its own scale and did not get one")
	}
	if !b.Tracks[1].Secondary || b.Tracks[0].Secondary {
		t.Error("the wrong track was moved off the shared scale")
	}
	// England's 1500 parcels must not set the scale the 52 staff are read on.
	if b.Axis2.Max >= b.YAxis.Max {
		t.Errorf("the scales are not independent: %v vs %v", b.Axis2.Max, b.YAxis.Max)
	}
}

func TestAComboOfOneMeasureIsRefused(t *testing.T) {
	err := load(`- kind: chart
  chart: combo
  title: t
  x: {field: region}
  metrics:
    - {field: parcels, draw: bar}`)
	if err == nil {
		t.Fatal("a combo of one measure is just that measure's own chart")
	}
}

func TestAFunnelFallsAndSaysByHowMuch(t *testing.T) {
	// Stages as columns, which is how most warehouses model a funnel.
	b := draw(t, `- kind: chart
  chart: funnel
  title: Conversion
  metrics:
    - {field: parcels, aggregate: sum, label: Handled}
    - {field: staff, aggregate: sum, label: Delivered}`)

	if len(b.Stages) != 2 {
		t.Fatalf("want a stage per metric, got %d", len(b.Stages))
	}
	if b.Stages[0].Share != 1 {
		t.Errorf("the first stage is %v of itself, want 1", b.Stages[0].Share)
	}
	if b.Stages[0].Drop != "" {
		t.Error("the first stage reported a fall it could not have had")
	}
	// 2200 parcels down to 77 staff.
	if b.Stages[1].Drop == "" {
		t.Error("the second stage did not say how far it fell")
	}
	if !(b.Stages[1].Share < b.Stages[0].Share) {
		t.Errorf("the funnel did not narrow: %v then %v", b.Stages[0].Share, b.Stages[1].Share)
	}
}

func TestAFunnelOverRowsComesBackLargestFirst(t *testing.T) {
	// Stages as rows of a dimension — the other honest shape. Ordered by the
	// compiler, because a funnel that is not descending is a bar chart.
	b := draw(t, `- kind: chart
  chart: funnel
  title: By region
  x: {field: region}
  y: {field: parcels, aggregate: sum}`)

	if len(b.Stages) != 2 {
		t.Fatalf("want a stage per region, got %d", len(b.Stages))
	}
	if b.Stages[0].Label != "England" {
		t.Errorf("stages came back %q first, want the largest", b.Stages[0].Label)
	}
}

func TestAWaterfallCarriesWhereEachBarFloats(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: waterfall
  title: Parcels
  x: {field: region}
  y: {field: parcels, aggregate: sum}`)

	// Two regions plus the closing total.
	if len(b.Steps) != 3 {
		t.Fatalf("want a step per region and a total, got %d", len(b.Steps))
	}
	if b.Steps[0].Start != 0 {
		t.Errorf("the first step starts at %v, want the baseline", b.Steps[0].Start)
	}
	// Each step picks up where the last left off, which is the arithmetic the
	// chart exists to have done already.
	for i := 1; i < len(b.Steps)-1; i++ {
		if b.Steps[i].Start != b.Steps[i-1].End {
			t.Errorf("step %d starts at %v, want %v", i, b.Steps[i].Start, b.Steps[i-1].End)
		}
	}
	last := b.Steps[len(b.Steps)-1]
	if !last.Total || last.Start != 0 || last.Value != 2200 {
		t.Errorf("the closing bar is wrong: %+v", last)
	}
	if last.Sign != 0 {
		t.Error("the total was given a sign, which would paint it as a rise")
	}
}

func TestAHeatmapFillsEveryPairAndMarksTheEmptyOnes(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: heatmap
  title: Parcels by region and carrier
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)

	// Two regions, two carriers: a full grid is four, and only three had rows.
	if len(b.Cells) != 4 {
		t.Fatalf("want a dense 2x2 grid, got %d cells", len(b.Cells))
	}
	if len(b.HeatRows) != 2 || len(b.HeatColumns) != 2 {
		t.Fatalf("axes are %v x %v", b.HeatRows, b.HeatColumns)
	}
	var empty int
	for _, c := range b.Cells {
		if c.Empty {
			empty++
		}
	}
	if empty != 1 {
		t.Errorf("%d cells marked empty, want the one Baltic never reached", empty)
	}
}

func TestAHeatmapNeedsItsSecondAxis(t *testing.T) {
	err := load(`- kind: chart
  chart: heatmap
  title: t
  x: {field: region}
  y: {field: parcels}`)
	if err == nil {
		t.Fatal("a heatmap with one dimension is a bar chart wearing squares")
	}
}

func TestAGaugeReadsAgainstAFixedTarget(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: gauge
  title: Parcels against plan
  y: {field: parcels, aggregate: sum}
  target: {value: 4400, label: Plan}`)

	if b.Gauge == nil {
		t.Fatal("a gauge rendered no gauge")
	}
	if b.Gauge.Value != 2200 || b.Gauge.Target != 4400 {
		t.Errorf("gauge = %v of %v, want 2200 of 4400", b.Gauge.Value, b.Gauge.Target)
	}
	if b.Gauge.Share != 0.5 {
		t.Errorf("share = %v, want 0.5", b.Gauge.Share)
	}
	if b.Gauge.TargetLabel != "Plan" {
		t.Errorf("target label = %q", b.Gauge.TargetLabel)
	}
	if b.Gauge.Over != "" {
		t.Error("a gauge at half its target reported an overshoot")
	}
}

func TestBeatingTheTargetIsSaidInWordsNotInTheArc(t *testing.T) {
	// An arc cannot draw 180% — it would wrap past its own start and read as
	// 80%, which is the opposite of the news.
	b := draw(t, `- kind: chart
  chart: gauge
  title: Parcels against plan
  y: {field: parcels, aggregate: sum}
  target: {value: 1100}`)

	if b.Gauge.Share != 1 {
		t.Errorf("share = %v, want it capped at 1", b.Gauge.Share)
	}
	if b.Gauge.Over == "" {
		t.Error("the gauge beat its target and did not say so")
	}
}

func TestATreemapLaysOutRectanglesThatFillTheBox(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: treemap
  title: Parcels by region
  x: {field: region}
  y: {field: parcels, aggregate: sum}`)

	if len(b.Rects) != 2 {
		t.Fatalf("want a rectangle per region, got %d", len(b.Rects))
	}
	area := 0.0
	for _, r := range b.Rects {
		if r.X < 0 || r.Y < 0 || r.X+r.W > 1.0001 || r.Y+r.H > 1.0001 {
			t.Errorf("%s is outside the unit square: %+v", r.Label, r)
		}
		area += r.W * r.H
	}
	// The rectangles tile the box, so their areas sum to it.
	if area < 0.999 || area > 1.001 {
		t.Errorf("the rectangles cover %v of the box, want all of it", area)
	}
	// Area is the encoding: England's 1500 must cover more than Scotland's 700.
	if b.Rects[0].Label != "England" {
		t.Errorf("largest first is the order squarify needs, got %q", b.Rects[0].Label)
	}
}

func TestANestedTreemapKeepsItsLeavesInsideTheirGroup(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: treemap
  title: Parcels by carrier within region
  x: {field: carrier}
  series: {field: region}
  y: {field: parcels, aggregate: sum}`)

	frames := map[string]run.Rect{}
	var leaves []run.Rect
	for _, r := range b.Rects {
		if r.Depth == 0 {
			frames[r.Label] = r
		} else {
			leaves = append(leaves, r)
		}
	}
	if len(frames) != 2 {
		t.Fatalf("want a frame per region, got %d", len(frames))
	}
	for _, leaf := range leaves {
		f, ok := frames[leaf.Group]
		if !ok {
			t.Fatalf("%s names a group that was never drawn: %q", leaf.Label, leaf.Group)
		}
		if leaf.X < f.X-1e-9 || leaf.Y < f.Y-1e-9 ||
			leaf.X+leaf.W > f.X+f.W+1e-9 || leaf.Y+leaf.H > f.Y+f.H+1e-9 {
			t.Errorf("%s escapes its group's box: leaf %+v frame %+v", leaf.Label, leaf, f)
		}
		if leaf.Slot != f.Slot {
			t.Errorf("%s takes slot %d, want its group's %d", leaf.Label, leaf.Slot, f.Slot)
		}
	}
}

func TestAHeatmapGridIsBounded(t *testing.T) {
	// The grid is dense, so its size is the product of two cardinalities while
	// ChartLimit only bounds their sum. Two hundred distinct pairs is a 200x200
	// grid — forty thousand cells — unless the axes are capped.
	//
	// This is the shape that reaches production as `GROUP BY customer_id,
	// invoice_id`: a block that looks reasonable and returns a payload nothing
	// can draw.
	s, rep := geoReport(t, `- kind: chart
  chart: heatmap
  title: Every pair
  x: {field: carrier}
  series: {field: region}
  y: {field: parcels, aggregate: sum}`)

	// Rewrite the dataset under it with one that has high cardinality on both
	// axes, which is cheaper than shipping a 200-row fixture.
	db, err := sql.Open("sqlite", fmt.Sprintf("file:heat%d?mode=memory&cache=shared", charts.Add(1)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(geoSchema); err != nil {
		t.Fatal(err)
	}
	for i := range run.HeatAxis + 20 {
		if _, err := db.Exec(
			`INSERT INTO depots VALUES (?, '', 0, 0, 0, 0, ?, 1, 1)`,
			fmt.Sprintf("r%03d", i), fmt.Sprintf("c%03d", i)); err != nil {
			t.Fatal(err)
		}
	}

	ds, err := yamlcodec.Loader{}.Dataset([]byte(geoDataset))
	if err != nil {
		t.Fatal(err)
	}
	s = run.New(datasets{"depots": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db),
		Builder:  query.NewBuilder(query.SQLite{}),
	}})

	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	b := v.Blocks[0]

	if len(b.HeatRows) > run.HeatAxis || len(b.HeatColumns) > run.HeatAxis {
		t.Errorf("axes are %dx%d, want both capped at %d",
			len(b.HeatRows), len(b.HeatColumns), run.HeatAxis)
	}
	if len(b.Cells) > run.HeatAxis*run.HeatAxis {
		t.Errorf("%d cells, want at most %d", len(b.Cells), run.HeatAxis*run.HeatAxis)
	}
	// Truncating silently would let somebody conclude the rest were zero.
	if b.Total <= len(b.Cells) {
		t.Errorf("total = %d against %d cells drawn — a truncated grid must say so",
			b.Total, len(b.Cells))
	}
}

func TestPrintedChartsCarryWhatPaperNeeds(t *testing.T) {
	// The paginated renderer draws marks and never branches on chart type, so
	// anything the drawing depends on has to be decided here. These are the
	// two that are invisible until somebody opens the PDF.
	for _, c := range []struct {
		name   string
		block  string
		square bool
		note   string
	}{{
		name: "a pie is drawn in a square box",
		// Across the full width it is an ellipse, and an ellipse encodes a
		// direction the data does not have.
		block: `- kind: chart
  chart: pie
  title: Pie
  x: {field: region}
  y: {field: parcels, aggregate: sum}`,
		square: true,
	}, {
		name: "and so is a gauge",
		block: `- kind: chart
  chart: gauge
  title: Gauge
  y: {field: parcels, aggregate: sum}
  target: {value: 4400}`,
		square: true,
	}, {
		name: "a bar is not",
		block: `- kind: chart
  chart: bar
  title: Bar
  x: {field: region}
  y: {field: parcels, aggregate: sum}`,
	}, {
		name: "a map says why it is not printed rather than leaving a gap",
		block: `- kind: chart
  chart: map
  title: Map
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map: {geometry: shape}`,
		note: "browser",
	}} {
		t.Run(c.name, func(t *testing.T) {
			printed := run.Printable(run.View{Blocks: []run.Block{draw(t, c.block)}})
			if len(printed) != 1 {
				t.Fatalf("want one printed chart, got %d", len(printed))
			}
			p := printed[0]
			if p.Square != c.square {
				t.Errorf("square = %v, want %v", p.Square, c.square)
			}
			if c.note != "" && !strings.Contains(p.Note, c.note) {
				t.Errorf("note = %q, want it to mention %q", p.Note, c.note)
			}
			// Never nil: a null marshals as `none`, which the template cannot
			// call `.len()` on — and that took the whole document down.
			if p.Marks == nil {
				t.Error("marks are nil, which reaches the template as none")
			}
		})
	}
}
