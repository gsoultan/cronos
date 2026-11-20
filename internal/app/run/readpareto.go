package run

import (
	"cmp"
	"slices"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// readPareto reads (category, value) into the two tracks a Pareto chart is:
// the values as columns, largest first, and the running share of the whole
// as a line against a scale of its own from nothing to all of it.
//
// Drawn as a combo is, by every renderer that draws one, because that is
// what it is — two measures over the same categories, one on a second scale.
// Only the reading differs: the line is worked out here from the columns
// rather than asked of the database.
//
// Sorted here as well as by the query: an author's sort: still orders the
// rows, and a running share of categories in any other order says nothing.
func readPareto(blk definition.Block, ds definition.Dataset, rows Rows) ([]Track, *Axis, *Axis, error) {
	bars, err := readSeries(rows, blk.X.Grain)
	if err != nil {
		return nil, nil, nil, err
	}
	slices.SortStableFunc(bars, func(a, b Bar) int { return cmp.Compare(b.Value, a.Value) })

	var whole float64
	for _, b := range bars {
		if b.Value > 0 {
			whole += b.Value
		}
	}
	running := make([]Bar, len(bars))
	var sum float64
	for i, b := range bars {
		if b.Value > 0 {
			sum += b.Value
		}
		share := 0.0
		if whole > 0 {
			share = sum / whole
		}
		running[i] = Bar{Label: b.Label, Value: share, Formatted: sharePercent.short(share)}
	}

	y := axis(append(valuesOf(bars), 0))
	shares := cumulativeAxis()
	return []Track{
		{Label: measureHeading(ds, blk.Y), Slot: 0, Draw: "bar", Bars: bars},
		{Label: "Cumulative share", Slot: 1, Draw: "line", Secondary: true, Bars: running},
	}, &y, &shares, nil
}

// sharePercent writes a running share: 80%, 96.5%.
var sharePercent = numberFormat{kind: "percent", decimals: -1}

// cumulativeAxis is a running share's scale, fixed rather than fitted: it
// ends at all of the whole, which the line always reaches, and a line that
// started at 42% on a scale from 40% would read as a slow start.
func cumulativeAxis() Axis {
	out := Axis{Min: 0, Max: 1}
	for _, v := range []float64{0, 0.25, 0.5, 0.75, 1} {
		out.Ticks = append(out.Ticks, Tick{At: v, Label: sharePercent.tick(v)})
	}
	return out
}

// measureHeading is what a measure is called in a key: its field's label, or
// its name where it has none — and for a count, what the rows are, which is
// the dataset: a count of ids is tickets, not "id".
func measureHeading(ds definition.Dataset, m definition.MeasureRef) string {
	if m.Aggregate == "count" {
		if ds.Title != "" {
			return ds.Title
		}
		return "Count"
	}
	if f, ok := ds.Field(m.Field); ok && f.Label != "" {
		return f.Label
	}
	return m.Field
}
