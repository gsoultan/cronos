package run

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// readCombo reads (bucket, m0, m1, …) into one Track per measure.
//
// The metrics come back in the order the author listed them, which is the
// order the compiler aliased them in — so the column at index i+1 is metric i,
// and neither end has to name them to agree.
func readCombo(blk definition.Block, rows Rows) ([]Track, Axis, *Axis, error) {
	width := len(blk.Metrics) + 1
	tracks := make([]Track, len(blk.Metrics))
	for i, m := range blk.Metrics {
		tracks[i] = Track{
			Label: m.Heading(), Slot: i, Draw: drawOf(m),
			Secondary: m.Secondary(), Bars: []Bar{},
		}
	}

	for rows.Next() {
		cells := make([]any, width)
		into := make([]any, width)
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := rows.Scan(into...); err != nil {
			return nil, Axis{}, nil, err
		}
		label := bucketLabel(cells[0], blk.X.Grain)
		for i := range tracks {
			v, _ := number(cells[i+1])
			tracks[i].Bars = append(tracks[i].Bars, Bar{
				Label: label, Value: v, Formatted: compact(v),
			})
		}
	}

	primary, secondary := scales(tracks)
	return tracks, primary, secondary, rows.Err()
}

// drawOf is the mark a metric asks for, defaulting to bars.
//
// Bars and not lines, because a combo with nothing said is read as "these are
// the quantities" — and a line implies a continuity between buckets that a
// measure nobody described may not have.
func drawOf(m definition.Metric) string {
	if m.Line() {
		return "line"
	}
	return "bar"
}

// scales builds the axis each track is read against.
//
// One axis unless a measure asked to leave it. Where two scales do end up on
// one plot, they are still computed here rather than in the viewer, so the
// ticks beside them are formatted by the engine that knew the currency.
func scales(tracks []Track) (Axis, *Axis) {
	var shared, apart []float64
	for _, t := range tracks {
		for _, b := range t.Bars {
			if t.Secondary {
				apart = append(apart, b.Value)
			} else {
				shared = append(shared, b.Value)
			}
		}
	}
	primary := axis(shared)
	if len(apart) == 0 {
		return primary, nil
	}
	second := axis(apart)
	return primary, &second
}

// readFunnel reads the stages of a funnel, in either shape it comes in.
func readFunnel(blk definition.Block, rows Rows) ([]Stage, error) {
	if len(blk.Metrics) > 0 {
		return funnelFromMetrics(blk, rows)
	}
	return funnelFromRows(blk, rows)
}

// funnelFromMetrics reads one row of N measures — stages that are columns.
func funnelFromMetrics(blk definition.Block, rows Rows) ([]Stage, error) {
	out := []Stage{}
	if !rows.Next() {
		// An aggregate over no rows still returns a row, so this is a query
		// that matched nothing at all rather than a funnel of zeroes.
		return out, rows.Err()
	}
	cells := make([]any, len(blk.Metrics))
	into := make([]any, len(blk.Metrics))
	for i := range cells {
		into[i] = &cells[i]
	}
	if err := rows.Scan(into...); err != nil {
		return nil, err
	}
	for i, m := range blk.Metrics {
		v, _ := number(cells[i])
		out = append(out, Stage{Label: m.Heading(), Value: v, Formatted: compact(v)})
	}
	return shares(out), rows.Err()
}

// funnelFromRows reads (stage, value) — stages that are rows of a dimension.
//
// Already ordered largest first by the compiler, which is what makes it a
// funnel rather than a bar chart lying on its side.
func funnelFromRows(blk definition.Block, rows Rows) ([]Stage, error) {
	out := []Stage{}
	for rows.Next() {
		var label, value any
		if err := rows.Scan(&label, &value); err != nil {
			return nil, err
		}
		v, _ := number(value)
		out = append(out, Stage{
			Label: bucketLabel(label, blk.X.Grain), Value: v, Formatted: compact(v),
		})
	}
	return shares(out), rows.Err()
}

// shares fills in each stage's width and the fall that reached it.
func shares(stages []Stage) []Stage {
	if len(stages) == 0 {
		return stages
	}
	top := stages[0].Value
	for i := range stages {
		if top > 0 {
			stages[i].Share = stages[i].Value / top
		}
		if i == 0 {
			// Nothing to have fallen from. A "100%" here would read as a
			// conversion rather than as the start.
			continue
		}
		prev := stages[i-1].Value
		if prev == 0 {
			// Everything after an empty stage is undefined rather than a
			// total loss: nobody reached it to drop out of.
			continue
		}
		stages[i].Drop = fmt.Sprintf("%.1f%%", (stages[i].Value/prev-1)*100)
	}
	return stages
}
