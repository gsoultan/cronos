package run

import (
	"slices"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

// table lays a crosstab out as the table every renderer already draws: the row
// dimension down the left, a column per value of the other, and a total on
// each side.
//
// A pivot reaches a viewer, a PDF and a workbook as a table because that is
// what it is once it has been crossed. Sending it as a kind of its own would
// have meant four renderers learning to draw a grid of strings they already
// draw.
func (x crosstab) table(blk definition.Block, ds definition.Dataset) ([]Column, [][]string) {
	cols := make([]Column, 0, len(x.columns)+3)
	cols = append(cols, Column{Label: labelOf(ds, blk.Pivot.Rows.Field)})
	for _, c := range x.columns {
		cols = append(cols, Column{Label: c, Align: "right"})
	}
	cols = append(cols, Column{Label: "Total", Align: "right"})

	nf := formatOf(ds, blk.Value.Field)
	out := make([][]string, 0, len(x.rows)+1)
	lines := make([][]float64, 0, len(x.rows)+1)
	for r, label := range x.rows {
		row := make([]string, 0, len(cols)+1)
		row = append(row, label)
		for c := range x.columns {
			row = append(row, pivotCell(nf, x.cells[r][c], x.nulls[[2]int{r, c}]))
		}
		out = append(out, append(row, pivotCell(nf, x.rowTotals[r], false)))
		lines = append(lines, sparkOf(folded(x.cells[r])))
	}
	if len(x.rows) > 0 {
		last := make([]string, 0, len(cols)+1)
		last = append(last, x.totalLabel())
		for _, f := range x.columnTotals {
			last = append(last, pivotCell(nf, f, false))
		}
		out = append(out, append(last, pivotCell(nf, x.total, false)))
		lines = append(lines, sparkOf(folded(x.columnTotals)))
	}
	if blk.Pivot.Shade {
		x.shade(cols)
	}
	if !blk.Pivot.Sparkline || len(x.columns) < 2 {
		return cols, out
	}
	return withTrend(cols, out, lines)
}

// withTrend puts the sparkline beside the name of the row it draws. A year of
// months is wider than a screen, and a line at the far end of it is the one
// thing on the table that is meant to be read before the figures.
func withTrend(cols []Column, rows [][]string, lines [][]float64) ([]Column, [][]string) {
	cols = slices.Insert(cols, 1, Column{Label: "Trend", Spark: lines})
	for i := range rows {
		rows[i] = slices.Insert(rows[i], 1, "")
	}
	return cols, rows
}

// folded is a row of folds as the line through them. A month nothing
// matched is a month of nothing, so it is drawn at nothing rather than
// skipped: a gap in a line of sales is a month with none.
func folded(row []fold) []float64 {
	out := make([]float64, len(row))
	for i, f := range row {
		if f.set {
			out[i] = f.value
		}
	}
	return out
}

// totalLabel heads the bottom row, and says so when the totals are of less
// than everything that matched.
func (x crosstab) totalLabel() string {
	if x.cut {
		return "Total of the first " + group(query.PivotLimit, 0) + " cells"
	}
	return "Total"
}

// pivotCell prints one cell: its number, a dash for a measure that was null,
// and nothing at all where no row matched.
//
// Blank rather than zero, because a pivot is mostly read for its gaps — which
// customer bought nothing in March — and a zero there is a sale of nothing.
func pivotCell(nf numberFormat, f fold, null bool) string {
	switch {
	case f.set:
		return nf.exact(f.value)
	case null:
		return "—"
	}
	return ""
}

// labelOf is a field's label, or its name when it has none.
func labelOf(ds definition.Dataset, name string) string {
	if f, ok := ds.Field(name); ok && f.Label != "" {
		return f.Label
	}
	return name
}

// shade gives each column of cells its steps on the ramp, classed by quantile
// across every cell that holds a figure, as a heatmap's are. The row labels,
// the totals down the right and the totals row stay plain.
func (x crosstab) shade(cols []Column) {
	var values []float64
	for r := range x.rows {
		for c := range x.columns {
			if x.cells[r][c].set {
				values = append(values, x.cells[r][c].value)
			}
		}
	}
	breaks := steps(values)
	for c := range x.columns {
		shades := make([]int, 0, len(x.rows)+1)
		for r := range x.rows {
			f := x.cells[r][c]
			if !f.set {
				shades = append(shades, -1)
				continue
			}
			shades = append(shades, stepOf(f.value, breaks))
		}
		if len(x.rows) > 0 {
			shades = append(shades, -1)
		}
		cols[c+1].Shades = shades
	}
}
