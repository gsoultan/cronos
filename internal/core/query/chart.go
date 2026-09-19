package query

import (
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// ChartLimit caps the rows any chart returns.
//
// A chart groups, so its row count is a cardinality rather than a row count —
// but `GROUP BY customer_id` on a dataset with two million customers is a
// cardinality of two million, and the block that did it will have looked
// reasonable to whoever wrote it. The cap turns "the page never loaded" into
// a chart that is visibly truncated.
const ChartLimit = 5_000

// chartSQL compiles a chart block to its shape.
//
// Four shapes rather than one, because what a chart selects follows from its
// type: a bar groups a dimension, a scatter reads two measures per category,
// and a map reads coordinates. Each returns its columns in the order
// ChartColumns names, which is the contract the reader in internal/app/run
// reads them back by.
func (b Builder) chartSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	switch {
	case blk.Chart.Geographic():
		return b.mapSQL(ds, blk, inner)
	case blk.Chart.Plots():
		return b.plotSQL(ds, blk, inner)
	case blk.Chart.Folded():
		return b.gaugeSQL(ds, blk, inner)
	case len(blk.Metrics) > 0:
		return b.metricSQL(ds, blk, inner)
	}
	// A waterfall, a treemap and a heatmap are all this shape: the difference
	// between them is entirely in how the same buckets are drawn, which is the
	// point of splitting the chart type from the block kind.
	return b.seriesSQL(ds, blk, inner)
}

// metricSQL reads several measures at once.
//
// A combo groups them by a bucket; a funnel whose stages are separate columns
// does not group at all and comes back as one row. Both are the same SELECT
// with and without a GROUP BY, which is why they are one function.
//
// The measures are aliased m0, m1, … in the order the author listed them,
// because that order is the chart — a combo's legend and a funnel's stages
// both read down it.
func (b Builder) metricSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	sel := make([]string, 0, len(blk.Metrics)+1)
	var group []string

	if blk.X.Field != "" {
		x, err := column(ds, blk.X.Field)
		if err != nil {
			return "", err
		}
		if blk.X.Grain != "" {
			if x, err = b.dialect.Bucket(blk.X.Grain, x); err != nil {
				return "", err
			}
		}
		sel = append(sel, x+" AS bucket")
		group = append(group, x)
	}

	for i, m := range blk.Metrics {
		expr, err := b.measure(ds, m.Ref())
		if err != nil {
			return "", err
		}
		sel = append(sel, fmt.Sprintf("%s AS m%d", expr, i))
	}
	return b.wrap(sel, inner, blk, group, b.order(ds, blk, len(group)))
}

// gaugeSQL folds the whole set to one number and, where the target is a
// column, the number it is read against.
func (b Builder) gaugeSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	value, err := b.measure(ds, blk.Y)
	if err != nil {
		return "", err
	}
	sel := []string{value + " AS value"}

	// A fixed target is a number in the definition and never reaches SQL. The
	// alternative — interpolating it into the statement — would put a value
	// from a stored document into a query for no gain at all.
	if !blk.Target.Fixed() {
		target, err := b.measure(ds, blk.Target.Ref())
		if err != nil {
			return "", err
		}
		sel = append(sel, target+" AS target")
	}
	return b.wrap(sel, inner, blk, nil, orderedBy{})
}

// seriesSQL buckets and folds — the categorical charts.
func (b Builder) seriesSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	y, err := b.measure(ds, blk.Y)
	if err != nil {
		return "", err
	}
	x, err := column(ds, blk.X.Field)
	if err != nil {
		return "", err
	}
	if blk.X.Grain != "" {
		if x, err = b.dialect.Bucket(blk.X.Grain, x); err != nil {
			return "", err
		}
	}

	group := []string{x}
	sel := []string{x + " AS bucket"}
	if blk.Series.Field != "" {
		s, err := column(ds, blk.Series.Field)
		if err != nil {
			return "", err
		}
		sel = append(sel, s+" AS series")
		group = append(group, s)
	}
	sel = append(sel, y+" AS value")

	/*
	   The expressions again in GROUP BY, not ordinals.

	   This was `GROUP BY 1` and the comment beside it said every dialect here
	   accepts the ordinal, which was true of the three that existed. SQL Server
	   does not: it reads the 1 as a constant and answers "each GROUP BY
	   expression must contain at least one column that is not an outer
	   reference", which is a sentence nobody would connect to this line.

	   ORDER BY keeps its ordinals, which an ordinal is accepted in everywhere
	   including here. Ordering by the series too is what stops a grouped bar
	   chart drawing its categories in a different order per bucket.
	*/
	return b.wrap(sel, inner, blk, group, b.order(ds, blk, len(group)))
}

// plotSQL reads two measures per category — a scatter or a bubble.
//
// The dimension is what each dot *is*, which is why it is grouped by rather
// than selected raw: the alternative is one dot per row, and rows are the one
// thing a dataset does not bound.
func (b Builder) plotSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	label, err := column(ds, blk.X.Field)
	if err != nil {
		return "", err
	}
	x, err := b.measure(ds, blk.XValue)
	if err != nil {
		return "", err
	}
	y, err := b.measure(ds, blk.Y)
	if err != nil {
		return "", err
	}

	sel := []string{label + " AS label", x + " AS x", y + " AS y"}
	group := []string{label}
	if blk.Series.Field != "" {
		s, err := column(ds, blk.Series.Field)
		if err != nil {
			return "", err
		}
		sel = append(sel, s+" AS series")
		group = append(group, s)
	}
	if blk.Size.Field != "" {
		size, err := b.measure(ds, blk.Size)
		if err != nil {
			return "", err
		}
		sel = append(sel, size+" AS size")
	}
	return b.wrap(sel, inner, blk, group, b.order(ds, blk, len(group)))
}

// measure renders one MeasureRef as its aggregate applied to its column.
func (b Builder) measure(ds definition.Dataset, m definition.MeasureRef) (string, error) {
	fn, err := aggregateOf(ds, m)
	if err != nil {
		return "", err
	}
	col, err := column(ds, m.Field)
	if err != nil {
		return "", err
	}
	return fn + "(" + col + ")", nil
}

// wrap assembles the statement every chart shape shares.
//
// Always aliased, always grouped by expression, always capped. Postgres names
// a bare SUM() column "sum", MySQL names it "SUM(total)", and a reader keyed
// on either breaks on the other.
func (b Builder) wrap(sel []string, inner string, blk definition.Block,
	group []string, order orderedBy) (string, error) {

	if order.err != nil {
		return "", order.err
	}
	// A gauge and a column-per-stage funnel fold the whole set, so there is
	// nothing to group by — and an empty GROUP BY clause is a syntax error in
	// every dialect here rather than a no-op.
	by := ""
	if len(group) > 0 {
		by = "\nGROUP BY " + strings.Join(group, ", ")
	}
	top, tail := b.dialect.Limit(ChartLimit)
	return fmt.Sprintf("SELECT %s%s\nFROM (\n%s\n) AS %s%s%s%s%s",
		top, strings.Join(sel, ", "), inner, blockAlias, where(blk.Filter),
		by, order.sql, tail), nil
}

// orderedBy is an ORDER BY clause or the reason there is not one, so the
// ordering can be chosen before the caller is in a position to return an error.
type orderedBy struct {
	sql string
	err error
}

// order picks the ordering: the author's, or the one the chart type means.
//
// A funnel and a treemap are read largest-first — a funnel because that is what
// makes it a funnel, and a treemap because the squarified layout it feeds wants
// its rectangles in descending order. Everything else reads along its buckets,
// which for a date is chronological and for a waterfall is the sequence the
// running total accumulates in.
func (b Builder) order(ds definition.Dataset, blk definition.Block, groups int) orderedBy {
	if len(blk.Sort) > 0 {
		sql, err := orderBy(ds, blk.Sort)
		return orderedBy{sql: sql, err: err}
	}
	if blk.Chart.Ordinal() && groups > 0 {
		// By the value, which is the column after the grouping expressions.
		// An ordinal, because an aggregate repeated in ORDER BY is a second
		// place for it to drift out of step with the SELECT.
		return orderedBy{sql: fmt.Sprintf("\nORDER BY %d DESC", groups+1)}
	}
	if groups == 0 {
		return orderedBy{}
	}
	parts := make([]string, groups)
	for i := range parts {
		parts[i] = fmt.Sprint(i + 1)
	}
	return orderedBy{sql: "\nORDER BY " + strings.Join(parts, ", ")}
}
