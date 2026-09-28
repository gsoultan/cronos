package run_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
 * The layers maps gained after the first five, end to end against the same
 * depots as chart_test.go: three depots, two carriers, two regions, and a route
 * out of each — a LineString, a LineString, and a MultiLineString.
 */

func TestALineLayerStrokesEachRouteAndShadesItByValue(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: map
  title: Routes
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  map: {layers: [line], geometry: route}`)

	if len(b.Map.Lines) != 3 {
		t.Fatalf("want a line per route, got %d", len(b.Map.Lines))
	}
	if len(b.Map.Shapes) != 0 {
		t.Errorf("a line layer filled %d shapes", len(b.Map.Shapes))
	}
	byName := map[string]run.Shape{}
	for _, l := range b.Map.Lines {
		byName[l.Label] = l
		if !strings.HasPrefix(l.Path, "M") || strings.Contains(l.Path, "Z") {
			t.Errorf("%s is not an open path: %q", l.Label, l.Path)
		}
	}
	// A MultiLineString is one route drawn as two strokes.
	if got := strings.Count(byName["Edinburgh"].Path, "M"); got != 2 {
		t.Errorf("Edinburgh's two legs became %d subpaths", got)
	}
	if byName["London"].Step <= byName["Bristol"].Step {
		t.Errorf("London (1,200) is not shaded above Bristol (300): %d vs %d",
			byName["London"].Step, byName["Bristol"].Step)
	}
	if len(b.Map.Legend) == 0 {
		t.Error("a shaded line layer came with no legend")
	}
}

// One field, two layers: the polygon layer shades areas and nothing else.
// Pointing it at lines is the author naming the wrong column, which a map
// with nothing drawn on it would never tell them.
func TestAPolygonLayerOverLinesSaysWhatIsWrong(t *testing.T) {
	s, rep := geoReport(t, `- kind: chart
  chart: map
  title: Routes
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  map: {layers: [polygon], geometry: route}`)

	_, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err == nil || !strings.Contains(err.Error(), "add a line layer") {
		t.Fatalf("lines under a polygon layer drew silently: %v", err)
	}
}

func TestHexagonsFoldThePointsInsideThemWithTheMeasuresOwnAggregate(t *testing.T) {
	for _, c := range []struct {
		aggregate string
		want      func(values []float64) float64
		total     float64
	}{
		{"sum", sum, 1200 + 300 + 700},
		{"max", maxOf, 1200},
	} {
		t.Run(c.aggregate, func(t *testing.T) {
			b := draw(t, `- kind: chart
  chart: map
  title: Density
  x: {field: depot}
  y: {field: parcels, aggregate: `+c.aggregate+`}
  map: {layers: [hexbin], lat: lat, lon: lon}`)

			var values []float64
			located := 0
			for _, h := range b.Map.Hexes {
				values = append(values, h.Value)
				n, _ := strconv.Atoi(strings.Fields(h.Label)[0])
				located += n
				if strings.Count(h.Path, "L") != 5 || !strings.HasSuffix(h.Path, "Z") {
					t.Errorf("a hexagon has the wrong number of sides: %q", h.Path)
				}
			}
			if got := c.want(values); got != c.total {
				t.Errorf("hexagons fold to %v, want %v", got, c.total)
			}
			if located != 3 {
				t.Errorf("hexagons hold %d depots between them, want 3", located)
			}
			if len(b.Map.Markers) != 0 {
				t.Errorf("%d markers were sent for a map that draws none", len(b.Map.Markers))
			}
		})
	}
}

// A hexagon width in kilometres is a width in kilometres: at the latitude of
// the data, flat side to flat side.
func TestAHexagonIsAsWideAsTheAuthorSaid(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: map
  title: Density
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  map: {layers: [hexbin], lat: lat, lon: lon, hexKm: 50}`)

	if len(b.Map.Hexes) != 3 {
		t.Fatalf("three depots hundreds of kilometres apart fell into %d hexagons", len(b.Map.Hexes))
	}
	xs := vertices(t, b.Map.Hexes[0].Path, 0)
	width := extent(xs)
	ys := vertices(t, b.Map.Hexes[0].Path, 1)
	lat := latitude((ys[0] + ys[len(ys)-1]) / 2)
	km := width * 40_075.017 * math.Cos(lat*math.Pi/180)
	// The hexagon is sized at the middle of the data, not at its own row.
	if km < 45 || km > 55 {
		t.Errorf("a 50 km hexagon is %.1f km wide", km)
	}
}

func TestPointsAreColouredByCategoryAndTheColoursAreNamed(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: map
  title: By carrier
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  series: {field: carrier}
  map: {layers: [scatter], lat: lat, lon: lon}`)

	slots := map[string]int{}
	for _, m := range b.Map.Markers {
		slots[m.Label] = m.Slot
	}
	if slots["London"] != slots["Edinburgh"] || slots["London"] == slots["Bristol"] {
		t.Errorf("colours do not follow the carrier: %v", slots)
	}
	if len(b.Map.Keys) != 2 || b.Map.Keys[0].Label == b.Map.Keys[1].Label {
		t.Fatalf("keys = %+v, want one per carrier", b.Map.Keys)
	}
	for _, k := range b.Map.Keys {
		if k.Label == "Aurora" && k.Slot != slots["London"] {
			t.Errorf("the legend says Aurora is slot %d and London is drawn in %d", k.Slot, slots["London"])
		}
	}
}

func TestFlowsAloneSendNoMarkers(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: map
  title: Transfers
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  series: {field: carrier}
  map: {layers: [flow], lat: lat, lon: lon, toLat: to_lat, toLon: to_lon}`)

	if len(b.Map.Arcs) != 3 || len(b.Map.Markers) != 0 {
		t.Fatalf("arcs = %d, markers = %d; want 3 and none", len(b.Map.Arcs), len(b.Map.Markers))
	}
	if b.Map.Arcs[0].Slot == b.Map.Arcs[1].Slot {
		t.Errorf("London (Aurora) and Bristol (Baltic) flows share a colour: %+v", b.Map.Arcs)
	}
}

// basemaps is a run.Basemaps that answers with whatever it was given.
type basemaps struct {
	calls int
	tiles *run.Tiles
	err   error
}

func (b *basemaps) Tiles(context.Context, definition.Basemap, run.Bounds) (*run.Tiles, error) {
	b.calls++
	return b.tiles, b.err
}

const twoOutputs = `
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: depot-map}
spec:
  dataset: depots
  outputs:
    - name: interactive
      renderer: interactive
      layout: &layout
        - kind: chart
          chart: map
          title: Depots
          x: {field: depot}
          y: {field: parcels, aggregate: sum}
          map:
            layers: [scatter]
            lat: lat
            lon: lon
            basemap: {provider: mapbox, style: light}
    - name: pdf
      renderer: paginated
      layout: *layout
`

// A basemap can mean a key and a round trip to Google, and paper prints the
// data without one — so a five-thousand-statement burst must not resolve it
// five thousand times.
func TestABasemapIsResolvedForABrowserAndNotForPaper(t *testing.T) {
	s, _ := geoReport(t, `- kind: text
  text: placeholder`)
	rep, err := yamlcodec.Loader{}.Report([]byte(twoOutputs))
	if err != nil {
		t.Fatal(err)
	}
	port := &basemaps{tiles: &run.Tiles{URL: "https://tiles.test/{z}/{x}/{y}.png"}}
	s.WithBasemaps(port)

	view, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	if port.calls != 1 || view.Blocks[0].Map.Tiles == nil {
		t.Fatalf("the browser's map was not given its basemap (%d calls)", port.calls)
	}
	if _, err := s.Render(context.Background(), rep, run.Request{Output: "pdf"}, anyone()); err != nil {
		t.Fatal(err)
	}
	if port.calls != 1 {
		t.Errorf("rendering for paper resolved the basemap too")
	}
}

func TestAMapWhoseBasemapCannotBeDrawnIsStillDrawn(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		says string
	}{
		{"a reason for the reader", run.Unavailable{Reason: "Mapbox is not set up on this server."},
			"Mapbox is not set up on this server."},
		// Anything else is a fault, and its text is not the reader's business.
		{"a fault", errors.New("dial tcp 10.0.0.7:443: connection refused"), "could not be loaded"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, _ := geoReport(t, `- kind: text
  text: placeholder`)
			rep, err := yamlcodec.Loader{}.Report([]byte(twoOutputs))
			if err != nil {
				t.Fatal(err)
			}
			s.WithBasemaps(&basemaps{err: c.err})
			view, err := s.Render(context.Background(), rep, run.Request{}, anyone())
			if err != nil {
				t.Fatalf("a basemap failed the whole report: %v", err)
			}
			m := view.Blocks[0].Map
			if m.Tiles != nil || len(m.Markers) != 3 {
				t.Errorf("tiles = %v, markers = %d; want no tiles and every depot", m.Tiles, len(m.Markers))
			}
			if !strings.Contains(m.Note, c.says) || strings.Contains(m.Note, "10.0.0.7") {
				t.Errorf("note = %q", m.Note)
			}
		})
	}
}

// A service with no provider wiring still draws what needs none.
func TestWithoutProvidersAURLBasemapStillDraws(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: map
  title: Depots
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  map:
    layers: [scatter]
    lat: lat
    lon: lon
    basemap:
      url: https://tiles.test/{z}/{x}/{y}{r}.png
      attribution: © Test`)

	if b.Map.Tiles == nil || b.Map.Tiles.URL2x != "https://tiles.test/{z}/{x}/{y}@2x.png" {
		t.Fatalf("tiles = %+v", b.Map.Tiles)
	}
}

func sum(values []float64) float64 {
	t := 0.0
	for _, v := range values {
		t += v
	}
	return t
}

func maxOf(values []float64) float64 {
	m := math.Inf(-1)
	for _, v := range values {
		m = math.Max(m, v)
	}
	return m
}

// vertices reads one coordinate of every vertex out of a path.
func vertices(t *testing.T, path string, axis int) []float64 {
	t.Helper()
	var out []float64
	for _, pair := range strings.FieldsFunc(path, func(r rune) bool { return r == 'M' || r == 'L' || r == 'Z' }) {
		xy := strings.Fields(pair)
		v, err := strconv.ParseFloat(xy[axis], 64)
		if err != nil {
			t.Fatalf("path %q: %v", path, err)
		}
		out = append(out, v)
	}
	return out
}

// extent is how far a set of values spans.
func extent(vs []float64) float64 {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range vs {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return hi - lo
}

// latitude is the projection's inverse for y.
func latitude(y float64) float64 {
	return math.Atan(math.Sinh(math.Pi*(1-2*y))) * 180 / math.Pi
}

// A break interpolated between two regions is a number nobody chose. The
// legend used to print it whole — "833.33–966.67" between two whole counts of
// parcels — and on a hexbin a break a hair over 36 printed as "36.00".
func TestALegendReadsInRoundNumbers(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: map
  title: Parcels by region
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map: {geometry: shape}`)

	if len(b.Map.Legend) < 2 {
		t.Fatalf("legend = %+v, want bands between 700 and 1,500", b.Map.Legend)
	}
	for _, l := range b.Map.Legend {
		for _, s := range []string{l.From, l.To} {
			if strings.Contains(s, ".") {
				t.Errorf("a band reads %q–%q", l.From, l.To)
			}
		}
	}
}

// depotsWith is the depots dataset over a table holding only the rows given,
// for a test that needs a shape of data the shared three depots do not have.
func depotsWith(t *testing.T, rows string) *run.Service {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:depots%d?mode=memory&cache=shared", charts.Add(1)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	create := geoSchema[:strings.Index(geoSchema, "INSERT")]
	if _, err := db.Exec(create + rows); err != nil {
		t.Fatal(err)
	}
	ds, err := yamlcodec.Loader{}.Dataset([]byte(geoDataset))
	if err != nil {
		t.Fatal(err)
	}
	return run.New(datasets{"depots": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db),
		Builder:  query.NewBuilder(query.SQLite{}),
	}})
}

// renderOne renders a report of one block with s.
func renderOne(t *testing.T, s *run.Service, block string) run.Block {
	t.Helper()
	_, rep := geoReport(t, block)
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return v.Blocks[0]
}

// grid is ChartLimit + extra drops spread over a few square kilometres.
func grid(extra int) string {
	return fmt.Sprintf(`INSERT INTO depots
WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < %d)
SELECT 'England', '', 51.5 + (i %% 97) * 0.0003, -0.12 + (i / 97) * 0.0005, 0, 0,
       'Aurora', 1, 1, 'd' || i, '' FROM n;`, query.ChartLimit+extra)
}

/*
A map with more places than one payload holds is not cut at the cap.

It used to be: capped at ChartLimit like every chart, and a map folds its rows
into totals, so one cut at the cap was not a smaller picture of the data but a
wrong one — and after that it said so, which was honest and still wrong. Now it
is asked again, and the database folds every place.
*/
func TestAMapWithMorePlacesThanTheCapCountsThemAll(t *testing.T) {
	hexes := `- kind: chart
  chart: map
  title: Drops
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  map: {layers: [hexbin], lat: lat, lon: lon}`
	dots := strings.Replace(hexes, "[hexbin]", "[scatter]", 1)

	whole := renderOne(t, depotsWith(t, grid(0)), dots)
	if whole.Map.Cells != nil || len(whole.Map.Markers) != query.ChartLimit {
		t.Errorf("a map of exactly the cap was gathered: %d markers, cells %v",
			len(whole.Map.Markers), whole.Map.Cells != nil)
	}

	s := depotsWith(t, grid(1))
	over := renderOne(t, s, hexes)
	total := 0.0
	for _, h := range over.Map.Hexes {
		total += h.Value
	}
	if total != query.ChartLimit+1 || over.Map.Partial != "" {
		t.Errorf("the hexagons hold %v and say %q, want every one of the %d places and no caveat",
			total, over.Map.Partial, query.ChartLimit+1)
	}

	d := renderOne(t, s, dots)
	if d.Map.Cells == nil || len(d.Map.Markers) != 0 {
		t.Fatalf("a dot map past the cap came back as %d markers", len(d.Map.Markers))
	}
	n := 0
	for _, c := range d.Map.Cells.N {
		n += c
	}
	if n != query.ChartLimit+1 || d.Map.Places != query.ChartLimit+1 || d.Map.Partial != "" {
		t.Errorf("cells hold %d places and say %d, %q", n, d.Map.Places, d.Map.Partial)
	}
	if d.Map.Detail == nil || d.Map.Detail.Output != "interactive" || d.Map.Detail.Block != 0 {
		t.Errorf("a large map does not say how to ask for more of it: %+v", d.Map.Detail)
	}
}

// The width in kilometres holds wherever the data's middle is; a filter
// that moves the middle a little must not resize the grid under it.
func TestAFilterLeavesTheHexagonsWhereTheyWere(t *testing.T) {
	block := `- kind: chart
  chart: map
  title: Drops
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  map: {layers: [hexbin], lat: lat, lon: lon, hexKm: 50}`
	london := func(b run.Block) string {
		for _, h := range b.Map.Hexes {
			if h.Value == 1200 || h.Value == 1500 {
				return h.Path
			}
		}
		return ""
	}
	all := draw(t, block)
	aurora := draw(t, strings.Replace(block, "title: Drops", "title: Drops\n  filter: carrier = 'Aurora'", 1))
	if london(all) == "" || london(all) != london(aurora) {
		t.Errorf("London's hexagon moved when Bristol was filtered away:\n  %s\n  %s", london(all), london(aurora))
	}
}

// Only the flows are drawn, so a category with no flow on the map has
// nothing coloured in and no place in the legend.
func TestAFlowMapNamesOnlyTheCategoriesItDraws(t *testing.T) {
	s := depotsWith(t, `INSERT INTO depots VALUES
  ('England', '', 51.5074, -0.1278, 53.4808, -2.2426, 'Aurora', 1200, 40, 'London', ''),
  ('England', '', 51.4545, -2.5879, NULL, NULL, 'Baltic', 300, 12, 'Bristol', '');`)
	b := renderOne(t, s, `- kind: chart
  chart: map
  title: Transfers
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  series: {field: carrier}
  map: {layers: [flow], lat: lat, lon: lon, toLat: to_lat, toLon: to_lon}`)
	if len(b.Map.Arcs) != 1 || len(b.Map.Keys) != 1 || b.Map.Keys[0].Label != "Aurora" {
		t.Errorf("arcs = %d, keys = %+v; want Aurora's one flow and Aurora alone", len(b.Map.Arcs), b.Map.Keys)
	}
}

// A place is sent to the precision its region's outline is, and its weight to
// four decimals: seventeen digits of each were most of what a map of places
// sent, and none of them moved a pixel.
func TestAPlaceSendsNoDigitsAPixelCannotShow(t *testing.T) {
	blk := renderOne(t, depotsWith(t, `INSERT INTO depots VALUES
  ('A', '', 51.50735123456789, -0.12775812345678, 53.48075123456789, -2.24263412345678, 'Aurora', 7, 1, 'a', ''),
  ('B', '', 55.95325123456789, -3.18826712345678, 51.50735123456789, -0.12775812345678, 'Baltic', 3, 1, 'b', '');`),
		`- kind: chart
  chart: map
  title: Places
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  map: {layers: [bubble, flow], lat: lat, lon: lon, toLat: to_lat, toLon: to_lon}`)
	raw, err := json.Marshal(blk.Map)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`"(x|y|x1|y1|x2|y2|weight)":(-?[0-9.]+)`).FindAllStringSubmatch(string(raw), -1) {
		most := 8
		if m[1] == "weight" {
			most = 4
		}
		if _, frac, ok := strings.Cut(m[2], "."); ok && len(frac) > most {
			t.Errorf("%s is sent as %s, %d decimals where %d show everything", m[1], m[2], len(frac), most)
		}
	}
}

// A large map's cells are sent on the same grid, and without the zeros a fixed
// count pads with: nine decimals of every cell was a tenth of a pixel no
// screen draws.
func TestACellIsSentOnThePathGrid(t *testing.T) {
	raw, err := json.Marshal(run.Cells{Size: 0.5, X: []float64{0.123456789123, 0.5}, Y: []float64{0.987654321987, 0.25},
		N: []int{1, 1}, V: []float64{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"x":[0.12345679,0.5]`) || !strings.Contains(string(raw), `"y":[0.98765432,0.25]`) {
		t.Errorf("cells sent as %s", raw)
	}
}
