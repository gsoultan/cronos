package run

import (
	"math"
	"slices"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// dataBars works out the bars of the columns a table asked for, from the
// figures it read: each over the largest, by size, in its column.
//
// The largest on the rows sent, which on a table cut at its page size is the
// largest of the page. A bar is read against the bars beside it, and those are
// the ones on the page.
func dataBars(cols []Column, blk definition.Block, figures [][]*float64) {
	for i := range cols {
		if i >= len(blk.Columns) || !slices.Contains(blk.Bars, blk.Columns[i]) {
			continue
		}
		var most float64
		for _, row := range figures {
			if row[i] != nil {
				most = max(most, math.Abs(*row[i]))
			}
		}
		bars := make([]float64, len(figures))
		for r, row := range figures {
			if row[i] != nil && most > 0 {
				bars[r] = round4(*row[i] / most)
			}
		}
		cols[i].Bars = bars
	}
}

// sparkOf is one row's line: its points at 0 to 1 between its own lowest and
// highest. A row that never moves is drawn flat across the middle, rather
// than along the floor where it reads as nothing.
func sparkOf(values []float64) []float64 {
	if len(values) == 0 {
		return []float64{}
	}
	lo, hi := slices.Min(values), slices.Max(values)
	out := make([]float64, len(values))
	for i, v := range values {
		out[i] = 0.5
		if hi > lo {
			out[i] = round4((v - lo) / (hi - lo))
		}
	}
	return out
}

// round4 keeps four places, which is a hundredth of a percent of the cell —
// finer than anything drawn — and keeps the payload from carrying seventeen.
func round4(f float64) float64 { return math.Round(f*1e4) / 1e4 }

// figuresOf is a row's cells as numbers, nil where a cell is not one.
func figuresOf(cells []any) []*float64 {
	out := make([]*float64, len(cells))
	for i, c := range cells {
		if n, ok := number(c); ok {
			out[i] = &n
		}
	}
	return out
}
