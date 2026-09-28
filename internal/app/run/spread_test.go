package run_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/document"
)

/*
Histograms and box plots over the depots: parcels of 1,200 and 300 in England
and 700 in Scotland. Read in a real database, because the claims are about SQL
— that the bins fall on round numbers and every row lands in one, that the
quartiles are what the ranks say they are.
*/

// The range 300–1,200 cut into about twelve is nine bins of a hundred, each
// row in one — the largest on the last bin's upper edge, not past it.
func TestAHistogramBinsOnRoundNumbers(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: histogram
  title: Depot sizes
  x: {field: parcels}`)
	if len(b.Bins) != 9 || b.Bins[0].From != 300 || b.Bins[0].To != 400 || b.Bins[8].To != 1200 {
		t.Fatalf("bins %+v, want nine of a hundred from 300", b.Bins)
	}
	counts := map[int]float64{0: 1, 4: 1, 8: 1}
	for i, bin := range b.Bins {
		if bin.Value != counts[i] {
			t.Errorf("bin %s holds %v, want %v", bin.Label, bin.Value, counts[i])
		}
	}
	if b.Bins[0].Label != "300–400" || b.XAxis == nil || b.YAxis == nil || b.YAxis.Min != 0 {
		t.Fatalf("label %q, scales %+v %+v", b.Bins[0].Label, b.XAxis, b.YAxis)
	}
	// The scale reads to the range's end, whatever edges it skips on the way.
	if ticks := b.XAxis.Ticks; ticks[0].Label != "300" || ticks[len(ticks)-1].Label != "1,200" || ticks[len(ticks)-1].At != 1 {
		t.Errorf("ticks %+v, want 300 to 1,200", ticks)
	}
	out := marshal(t, b)
	for _, k := range []string{"bins", "xAxis", "yAxis", "series"} {
		if _, ok := out[k]; !ok {
			t.Errorf("%q is missing from a histogram's payload: %v", k, keys(out))
		}
	}
}

// With a measure, a bin says the measure folded over its rows.
func TestAHistogramFoldsItsMeasure(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: histogram
  title: Staff by depot size
  bins: 3
  x: {field: parcels}
  y: {field: staff, aggregate: sum}`)
	var total float64
	for _, bin := range b.Bins {
		total += bin.Value
	}
	if len(b.Bins) < 2 || total != 77 {
		t.Errorf("%d bins summing to %v, want the 77 staff across them", len(b.Bins), total)
	}
}

// A box per region: England's two depots span 300 to 1,200 with a median
// between them; Scotland's one is all 700.
func TestABoxPlotReadsTheQuartilesInTheDatabase(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: boxplot
  title: Depot sizes by region
  x: {field: region}
  y: {field: parcels}`)
	if len(b.Boxes) != 2 {
		t.Fatalf("%d boxes, want one a region", len(b.Boxes))
	}
	england, scotland := b.Boxes[0], b.Boxes[1]
	if england.Label != "England" || england.N != 2 || england.Low != 300 || england.High != 1200 ||
		england.Median != 750 || england.Q1 != 300 || england.Q3 != 1200 {
		t.Errorf("England %+v", england)
	}
	if scotland.Median != 700 || scotland.Q1 != 700 || scotland.N != 1 || scotland.Said[2] != "700" {
		t.Errorf("Scotland %+v", scotland)
	}
	if b.YAxis == nil || b.YAxis.Max < 1200 {
		t.Errorf("scale %+v", b.YAxis)
	}
	if out := marshal(t, b); out["boxes"] == nil {
		t.Errorf("the payload has no boxes: %v", keys(out))
	}
}

// Without x, one box for every row: 300, 700 and 1,200.
func TestABoxPlotWithoutCategoriesIsOneBox(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: boxplot
  title: Depot sizes
  y: {field: parcels}`)
	if len(b.Boxes) != 1 || b.Boxes[0].Median != 700 || b.Boxes[0].N != 3 || b.Boxes[0].Label != "" {
		t.Errorf("boxes %+v", b.Boxes)
	}
}

func TestWhatTheSpreadChartsRefuse(t *testing.T) {
	for _, c := range []struct{ name, block, says string }{
		{"a histogram bins a field", `- kind: chart
  chart: histogram
  title: t`, "bins no field"},
		{"a histogram does not bin a date by a grain", `- kind: chart
  chart: histogram
  title: t
  x: {field: day, grain: month}`, "column chart"},
		{"a histogram's bins are few enough to read", `- kind: chart
  chart: histogram
  title: t
  bins: 500
  x: {field: parcels}`, "want 2 to"},
		{"only a histogram has bins", `- kind: chart
  chart: bar
  title: t
  bins: 5
  x: {field: region}
  y: {field: parcels}`, "bins"},
		{"a box plot reads rows, not sums", `- kind: chart
  chart: boxplot
  title: t
  y: {field: parcels, aggregate: sum}`, "no aggregate"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := load(c.block)
			if !errors.Is(err, definition.ErrInvalid) || !strings.Contains(err.Error(), c.says) {
				t.Errorf("err = %v, want it refused saying %q", err, c.says)
			}
		})
	}
}

// On paper as on screen: a histogram's bins stand side by side along their
// range, and a box plot is a box, a median and two whiskers a category.
func TestTheSpreadChartsPrint(t *testing.T) {
	h := run.Printable(run.View{Blocks: []run.Block{draw(t, `- kind: chart
  chart: histogram
  title: Depot sizes
  x: {field: parcels}`)}})[0]
	bins := marksOf(h, document.RectMark)
	if len(bins) != 3 || len(h.XTicks) == 0 || len(h.Ticks) == 0 {
		t.Errorf("%d bars along %+v", len(bins), h.XTicks)
	}
	box := run.Printable(run.View{Blocks: []run.Block{draw(t, `- kind: chart
  chart: boxplot
  title: Depot sizes by region
  x: {field: region}
  y: {field: parcels}`)}})[0]
	if len(marksOf(box, document.RectMark)) != 2 || len(marksOf(box, document.LineMark)) < 6 || len(box.XTicks) != 2 {
		t.Errorf("%d boxes, %d lines under %+v", len(marksOf(box, document.RectMark)),
			len(marksOf(box, document.LineMark)), box.XTicks)
	}
}
