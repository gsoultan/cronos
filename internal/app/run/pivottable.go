package run

import (
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
	cols := make([]Column, 0, len(x.columns)+2)
	cols = append(cols, Column{Label: labelOf(ds, blk.Pivot.Rows.Field)})
	for _, c := range x.columns {
		cols = append(cols, Column{Label: c, Align: "right"})
	}
	cols = append(cols, Column{Label: "Total", Align: "right"})

	out := make([][]string, 0, len(x.rows)+1)
	for r, label := range x.rows {
		row := make([]string, 0, len(cols))
		row = append(row, label)
		for c := range x.columns {
			row = append(row, pivotCell(x.cells[r][c], x.nulls[[2]int{r, c}]))
		}
		out = append(out, append(row, pivotCell(x.rowTotals[r], false)))
	}
	if len(x.rows) == 0 {
		return cols, out
	}

	last := make([]string, 0, len(cols))
	last = append(last, x.totalLabel())
	for _, f := range x.columnTotals {
		last = append(last, pivotCell(f, false))
	}
	return cols, append(out, append(last, pivotCell(x.total, false)))
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
func pivotCell(f fold, null bool) string {
	switch {
	case f.set:
		return group(f.value, decimalsFor(f.value))
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
