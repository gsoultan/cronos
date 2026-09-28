package run_test

import (
	"math"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/document"
)

/*
What a chart puts on paper, mark by mark — the half of a report nobody looks
at until it is in somebody's inbox. Each of these was wrong in a PDF while
the same chart was right on the screen.
*/

// printOne draws one block over the three depots and prints it.
func printOne(t *testing.T, block string) document.Chart {
	t.Helper()
	s, _ := geoReport(t, "- kind: text\n  text: placeholder")
	charts := run.Printable(run.View{Blocks: []run.Block{renderOne(t, s, block)}})
	if len(charts) != 1 {
		t.Fatalf("%d charts printed, want one", len(charts))
	}
	return charts[0]
}

func marksOf(c document.Chart, kind string) []document.Mark {
	var out []document.Mark
	for _, m := range c.Marks {
		if m.Kind == kind {
			out = append(out, m)
		}
	}
	return out
}

func words(c document.Chart) string {
	var out []string
	for _, m := range marksOf(c, document.TextMark) {
		out = append(out, m.Label)
	}
	return strings.Join(out, " | ")
}

// A gauge opens at the bottom, where its figures are. It was centred on nine
// o'clock — -0.5 of a turn where a pie's twelve o'clock is -0.25 — and every
// gauge printed as a "C" lying on its back.
func TestAPrintedGaugeOpensAtTheBottom(t *testing.T) {
	c := printOne(t, `- kind: chart
  chart: gauge
  title: Against plan
  y: {field: parcels, aggregate: sum}
  target: {value: 4400}`)
	track := marksOf(c, document.PolyMark)[0]
	start, top := track.Points[0], 1.0
	for _, p := range track.Points {
		top = math.Min(top, p[1])
	}
	if start[0] >= 0.5 || start[1] <= 0.5 || top > 0.05 {
		t.Errorf("the dial starts at %v and reaches %.2f from the top; want the lower left and the top", start, top)
	}
	if !strings.Contains(words(c), "2,200") || !strings.Contains(words(c), "50% of") {
		t.Errorf("the dial says %q, want its value and its share of the target", words(c))
	}
	// The reading is the figure the dial is about, set large and fitted to
	// the hole rather than at a label's size.
	if big := marksOf(c, document.TextMark)[0]; big.Size < 12 || big.W <= 0 || big.W > 0.72 {
		t.Errorf("the reading is set at %vpt across %v of the box", big.Size, big.W)
	}
	if !strings.Contains(c.Note, "4,400") {
		t.Errorf("under the dial: %q, want what it is measured against", c.Note)
	}
}

// A line split by a series printed as horizontal bars: the paper checked for
// groups before it checked for a line.
func TestASplitLinePrintsAsLines(t *testing.T) {
	c := printOne(t, `- kind: chart
  chart: line
  title: By carrier
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)
	if len(marksOf(c, document.LineMark)) != 2 || len(marksOf(c, document.RectMark)) != 0 {
		t.Errorf("%d lines and %d bars, want a line per carrier", len(marksOf(c, document.LineMark)), len(marksOf(c, document.RectMark)))
	}
	if len(c.XTicks) != 2 || c.XTicks[0].Label != "England" {
		t.Errorf("under the line: %+v, want the regions", c.XTicks)
	}
}

// A combo's line runs through the middle of its columns, and a measure on its
// own scale is read against a second one down the right in its colour. The
// line ran edge to edge while the columns stood in their buckets' middles —
// past the plot on the right — and the second scale was not on the page.
func TestAPrintedComboLinesUpAndCarriesItsSecondScale(t *testing.T) {
	c := printOne(t, `- kind: chart
  chart: combo
  title: Parcels and staff
  x: {field: region}
  metrics:
    - {field: parcels, aggregate: sum, label: Parcels, draw: bar}
    - {field: staff, aggregate: sum, label: Staff, draw: line, axis: secondary}`)
	cols, line := marksOf(c, document.RectMark), marksOf(c, document.LineMark)
	if len(cols) != 2 || len(line) != 1 {
		t.Fatalf("%d columns and %d lines", len(cols), len(line))
	}
	for i, p := range line[0].Points {
		if mid := cols[i].X + cols[i].W/2; math.Abs(p[0]-mid) > 0.01 {
			t.Errorf("the line's point %d is at %.3f and its column's middle at %.3f", i, p[0], mid)
		}
	}
	if len(c.Ticks2) == 0 || c.Tone2 != "series-2" {
		t.Errorf("second scale %+v in %q, want one in the line's colour", c.Ticks2, c.Tone2)
	}
}

// A bar carries its name in a column on the left and its number at its end,
// in a box as tall as its rows need.
func TestAPrintedBarSaysWhatItIsAndHowMuch(t *testing.T) {
	c := printOne(t, `- kind: chart
  chart: bar
  title: By region
  x: {field: region}
  y: {field: parcels, aggregate: sum}`)
	said := words(c)
	for _, want := range []string{"England", "Scotland", "1,500", "700"} {
		if !strings.Contains(said, want) {
			t.Errorf("the bars say %q, missing %q", said, want)
		}
	}
	if c.Height <= 0 || c.Height > 30 {
		t.Errorf("two bars asked for a box %vmm tall", c.Height)
	}
	// Short names, a narrow column: a fixed quarter of the box left the
	// bars a hand's width from their names on a landscape page.
	if bar := marksOf(c, document.RectMark)[0]; bar.X > 0.12 {
		t.Errorf("the bars start %.2f across, for names of eight letters", bar.X)
	}
}

// A waterfall writes each change over its column and threads the running
// total from one to the next.
func TestAPrintedWaterfallCarriesItsChanges(t *testing.T) {
	c := printOne(t, `- kind: chart
  chart: waterfall
  title: What moved
  x: {field: region}
  y: {field: parcels, aggregate: sum}`)
	cols := marksOf(c, document.RectMark)
	if len(marksOf(c, document.LineMark)) != len(cols)-1 || !strings.Contains(words(c), "2,200") {
		t.Errorf("%d columns, %d threads, words %q", len(cols), len(marksOf(c, document.LineMark)), words(c))
	}
}

// A heatmap's cells say their values, its rows are named down the left and
// its columns under it.
func TestAPrintedHeatmapIsReadable(t *testing.T) {
	c := printOne(t, `- kind: chart
  chart: heatmap
  title: Carrier by region
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)
	said := words(c)
	for _, want := range []string{"Aurora", "Baltic", "1,200", "300"} {
		if !strings.Contains(said, want) {
			t.Errorf("the grid says %q, missing %q", said, want)
		}
	}
	if len(c.XTicks) != 2 {
		t.Errorf("under the grid: %+v, want both regions", c.XTicks)
	}
}

// A donut carries its whole in its middle, and each slice's share in the key.
func TestAPrintedDonutCarriesItsWhole(t *testing.T) {
	c := printOne(t, `- kind: chart
  chart: donut
  title: Share
  x: {field: region}
  y: {field: parcels, aggregate: sum}`)
	if !strings.Contains(words(c), "2,200") || len(c.Keys) != 2 || !strings.Contains(c.Keys[0].Label, "%") {
		t.Errorf("middle %q, keys %+v", words(c), c.Keys)
	}
	if whole := marksOf(c, document.TextMark)[0]; whole.Size < 12 || whole.W <= 0 || whole.W > 0.58 {
		t.Errorf("the whole is set at %vpt across %v of the box, want it large and inside the hole", whole.Size, whole.W)
	}
	for _, s := range marksOf(c, document.PolyMark) {
		if s.Stroke <= 0 {
			t.Error("a slice with no edge: two neighbours read as one fill")
		}
	}
}

// A treemap names its groups, as the screen does across the top of each
// frame: its leaves were coloured by a group nothing on the page named.
func TestAPrintedTreemapNamesItsGroups(t *testing.T) {
	c := printOne(t, `- kind: chart
  chart: treemap
  title: Carrier by region
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)
	said := words(c)
	for _, want := range []string{"England", "Scotland", "Aurora"} {
		if !strings.Contains(said, want) {
			t.Errorf("the treemap says %q, missing %q", said, want)
		}
	}
	if c.Height <= 46 {
		t.Errorf("a treemap in a box %vmm tall, want room for its names", c.Height)
	}
}

// A scatter's points are points: two bubbles filled a printed chart.
func TestAPrintedBubbleLeavesRoomForTheOthers(t *testing.T) {
	c := printOne(t, `- kind: chart
  chart: bubble
  title: Parcels against staff
  x: {field: region}
  xValue: {field: staff, aggregate: sum}
  y: {field: parcels, aggregate: sum}
  size: {field: staff, aggregate: sum}`)
	if len(marksOf(c, document.DotMark)) != 2 {
		t.Fatalf("%d bubbles, want one a region", len(marksOf(c, document.DotMark)))
	}
	for _, d := range marksOf(c, document.DotMark) {
		// A share of the width; about a seventh of the box's height at most.
		if d.W > 0.03 {
			t.Errorf("%s is printed %.3f of the width across", d.Label, d.W)
		}
	}
}

// A map of somewhere tall prints its places where they can be seen. A dot's
// radius is a share of the box's width, Edinburgh to Bristol is a narrow box,
// and every map was drawn in a chart's 46mm: its places printed a fifth of a
// millimetre across.
func TestATallMapsPlacesAreSeenOnPaper(t *testing.T) {
	c := printOne(t, `- kind: chart
  chart: map
  title: Depots
  x: {field: depot}
  y: {field: parcels, aggregate: sum}
  series: {field: carrier}
  map: {layers: [cluster], lat: lat, lon: lon}`)
	if c.Aspect >= 1 {
		t.Fatalf("aspect %v: the three depots are taller than they are wide", c.Aspect)
	}
	for _, d := range marksOf(c, document.DotMark) {
		// The template draws a dot's radius as a share of the box's width,
		// which for a tall map is its height times its aspect.
		if mm := d.W * c.Height * c.Aspect; mm < 0.4 {
			t.Errorf("%s is printed %.2fmm across", d.Label, mm*2)
		}
	}
}

// A category with no name, or a figure with no formatting, prints its bar
// without the word: a text mark with nothing to say is refused, and it took
// the whole document with it.
func TestANamelessBarDoesNotTakeTheDocumentDown(t *testing.T) {
	charts := run.Printable(run.View{Blocks: []run.Block{{Kind: "chart", Chart: "bar", Title: "Unnamed",
		Series: []run.Bar{{Label: "", Value: 3}, {Label: "North", Value: 5, Formatted: "5"}}}}})
	doc := document.Document{
		Title: "Dashboard", Period: "July", Org: document.Org{Name: "Finance"},
		Page:   document.Page{Size: "a4", Orientation: "portrait", MarginMM: 18},
		Charts: charts,
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("a bar with no name took the document down: %v", err)
	}
	if len(marksOf(charts[0], document.RectMark)) != 2 {
		t.Errorf("%d bars printed, want both", len(marksOf(charts[0], document.RectMark)))
	}
}
