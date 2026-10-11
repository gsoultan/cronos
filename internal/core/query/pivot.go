package query

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// PivotLimit caps the cells a pivot reads.
//
// Larger than ChartLimit because a pivot's cells are what it shows rather than
// marks on a chart: two hundred customers across twelve months is 2,400, and a
// grid of them is a page of a report rather than a chart nobody could read.
// The compiled statement asks for one more, so the reader can tell a pivot that
// filled the cap from one that was cut by it.
const PivotLimit = 50_000

// pivotSQL reads every pair of the two dimensions with the measure folded
// over it, ordered down the rows and then across.
//
// The crossing itself happens in the reader: turning pairs into columns in SQL
// needs a column per value, which is a statement that depends on the data it
// reads and a different syntax in every dialect here.
//
// An average also brings its sum and its count. The total of a row of averages
// is not their average, nor anything else the cells can be folded into, and a
// Total column that is quietly the mean of means is the wrong number in the
// place people look for the right one.
func (b Builder) pivotSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	if blk.Pivot == nil {
		return "", fmt.Errorf("%w: a pivot block crosses nothing", ErrBadTemplate)
	}
	sel := make([]string, 0, 5)
	group := make([]string, 0, 2)
	for i, d := range []definition.DimensionRef{blk.Pivot.Rows, blk.Pivot.Columns} {
		x, err := b.dimension(ds, d)
		if err != nil {
			return "", err
		}
		sel = append(sel, x+" AS "+[]string{"pivot_row", "pivot_column"}[i])
		group = append(group, x)
	}

	value, err := b.measure(ds, blk.Value)
	if err != nil {
		return "", err
	}
	sel = append(sel, value+" AS value")
	if fn, _ := aggregateOf(ds, blk.Value); fn == "AVG" {
		col, _ := column(ds, blk.Value.Field)
		sel = append(sel, "SUM("+col+") AS part_sum", "COUNT("+col+") AS part_count")
	}
	return b.capped(sel, inner, blk, group, orderedBy{sql: "\nORDER BY 1, 2"}, PivotLimit+1)
}

// dimension renders a DimensionRef, bucketed when it names a grain.
func (b Builder) dimension(ds definition.Dataset, d definition.DimensionRef) (string, error) {
	x, err := column(ds, d.Field)
	if err != nil || d.Grain == "" {
		return x, err
	}
	return b.dialect.Bucket(d.Grain, x)
}
