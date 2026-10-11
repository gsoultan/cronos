package run

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

// PivotRows and PivotColumns cap the axes a pivot shows.
//
// Past them the rest of an axis is folded into one last line, "12 more", rather
// than dropped: a pivot's totals are the reason to read one, and a Total column
// that does not add up across the cells beside it is a report that looks wrong
// even when it is right. A dozen months is two dozen columns at most; two
// dozen columns is already wider than a landscape page.
const (
	PivotRows    = 500
	PivotColumns = 24
)

// crosstab is a pivot, read and crossed.
type crosstab struct {
	rows, columns []string
	cells         [][]fold
	// rowTotals runs down the right-hand side, columnTotals along the bottom,
	// and total sits where they meet.
	rowTotals, columnTotals []fold
	total                   fold
	// nulls marks the pairs that matched rows whose measure was null, which
	// print as a dash rather than as the blank of a pair nothing matched.
	nulls map[[2]int]bool
	// cut says the read stopped at query.PivotLimit, so the totals are of the
	// cells that were read rather than of everything that matched.
	cut bool
}

// pair is one row of a pivot's result: a cell before it is placed.
type pair struct {
	row, column string
	raw         any
	value       fold
	null        bool
}

// readPivot reads (row, column, value[, sum, count]) pairs and crosses them.
//
// Rows keep the order the query returned them in, which is ascending. Columns
// are put in order here, by what the database returned rather than by label:
// the pairs come down a row at a time, so the order columns are first seen in
// is whichever the first row happened to have, and "Apr 2026" sorts before
// "Jan 2026" as text.
func readPivot(blk definition.Block, ds definition.Dataset, rows Rows) (crosstab, error) {
	var pairs []pair
	var rowOrder []string
	seenRow, columnRaw := map[string]bool{}, map[string]any{}
	for rows.Next() {
		p, err := scanPair(blk, ds, rows)
		if err != nil {
			return crosstab{}, err
		}
		if len(pairs) == query.PivotLimit {
			return crossed(blk, ds, pairs, rowOrder, columnRaw, true), nil
		}
		if !seenRow[p.row] {
			seenRow[p.row] = true
			rowOrder = append(rowOrder, p.row)
		}
		columnRaw[p.column] = p.raw
		pairs = append(pairs, p)
	}
	return crossed(blk, ds, pairs, rowOrder, columnRaw, false), rows.Err()
}

func scanPair(blk definition.Block, ds definition.Dataset, rows Rows) (pair, error) {
	var r, c, v, sum, count any
	dest := []any{&r, &c, &v}
	averaged := aggregateIn(blk, ds) == "avg"
	if averaged {
		dest = append(dest, &sum, &count)
	}
	if err := rows.Scan(dest...); err != nil {
		return pair{}, err
	}
	p := pair{row: bucketLabel(r, blk.Pivot.Rows.Grain), column: bucketLabel(c, blk.Pivot.Columns.Grain), raw: c}
	value, ok := number(v)
	if !ok {
		return pair{row: p.row, column: p.column, raw: c, null: true}, nil
	}
	p.value = fold{value: value, set: true}
	if averaged {
		p.value.sum, _ = number(sum)
		p.value.count, _ = number(count)
	}
	return p, nil
}

// aggregateIn is the fold a pivot's measure was read with: the block's, or
// the field's when the block names none.
func aggregateIn(blk definition.Block, ds definition.Dataset) string {
	if blk.Value.Aggregate != "" {
		return blk.Value.Aggregate
	}
	f, _ := ds.Field(blk.Value.Field)
	return f.Aggregate
}

// crossed places the pairs on their axes, folding each axis's overflow into
// its last line, and folds the totals.
func crossed(blk definition.Block, ds definition.Dataset, pairs []pair,
	rowOrder []string, columnRaw map[string]any, cut bool) crosstab {

	columnOrder := make([]string, 0, len(columnRaw))
	for c := range columnRaw {
		columnOrder = append(columnOrder, c)
	}
	slices.SortFunc(columnOrder, func(a, b string) int {
		return compareRaw(columnRaw[a], columnRaw[b], a, b)
	})

	rowAt, rowLabels := placed(rowOrder, PivotRows)
	columnAt, columnLabels := placed(columnOrder, PivotColumns)
	out := crosstab{
		rows: rowLabels, columns: columnLabels, cut: cut, nulls: map[[2]int]bool{},
		cells:     make([][]fold, len(rowLabels)),
		rowTotals: make([]fold, len(rowLabels)), columnTotals: make([]fold, len(columnLabels)),
	}
	for i := range out.cells {
		out.cells[i] = make([]fold, len(columnLabels))
	}

	agg := aggregateIn(blk, ds)
	for _, p := range pairs {
		r, c := rowAt[p.row], columnAt[p.column]
		if p.null {
			out.nulls[[2]int{r, c}] = true
			continue
		}
		out.cells[r][c].merge(p.value, agg)
		out.rowTotals[r].merge(p.value, agg)
		out.columnTotals[c].merge(p.value, agg)
		out.total.merge(p.value, agg)
	}
	return out
}

// placed numbers an axis's labels, folding every label past the cap into the
// last place it has.
func placed(labels []string, most int) (map[string]int, []string) {
	at := make(map[string]int, len(labels))
	shown := labels
	if len(labels) > most {
		shown = append(slices.Clip(labels[:most-1]), fmt.Sprintf("%d more", len(labels)-most+1))
	}
	for i, l := range labels {
		at[l] = min(i, len(shown)-1)
	}
	return at, shown
}

// compareRaw orders two values as the database returned them: numbers by
// size, dates by time, and anything else by its label.
func compareRaw(a, b any, la, lb string) int {
	if x, ok := number(a); ok {
		if y, ok := number(b); ok {
			return cmp.Compare(x, y)
		}
	}
	if x, ok := asTime(a); ok {
		if y, ok := asTime(b); ok {
			return x.Compare(y)
		}
	}
	return cmp.Compare(la, lb)
}
