package query

import (
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// blockAlias names the compiled dataset when a block aggregates over it.
const blockAlias = "cronos_block"

// BuildBlock compiles one block of a report into a plan for pr.
//
// Aggregation is pushed into SQL rather than folded in Go. A stat tile is one
// number: reading four million rows across the wire to add them up is the
// difference between a report that opens and one that times out, and the
// database was going to be better at it anyway.
//
// The shape wraps the dataset plan, so row scope is already applied to
// everything it can see. A block cannot aggregate over rows the caller was not
// allowed to read, because by the time this SQL runs those rows are not in the
// subquery.
func (b Builder) BuildBlock(ds definition.Dataset, blk definition.Block, in map[string]any,
	f Filters, pr principal.Principal) (Plan, Coverage, error) {

	base, cov, err := b.BuildWith(ds, in, f, pr)
	if err != nil {
		return Plan{}, Coverage{}, err
	}

	shaped, err := b.shape(ds, blk, base.sql)
	if err != nil {
		return Plan{}, Coverage{}, err
	}
	return Plan{sql: shaped, args: base.args}, cov, nil
}

func (b Builder) shape(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	switch blk.Kind {
	case definition.StatBlock:
		return b.statSQL(ds, blk, inner)
	case definition.ChartBlock:
		return b.chartSQL(ds, blk, inner)
	case definition.TableBlock:
		return b.tableSQL(ds, blk, inner)
	}
	return "", fmt.Errorf("%w: %q blocks are not compiled", ErrBadTemplate, blk.Kind)
}

// statSQL folds the whole set to one number.
func (b Builder) statSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	fn, err := aggregateOf(ds, blk.Value)
	if err != nil {
		return "", err
	}
	field, err := column(ds, blk.Value.Field)
	if err != nil {
		return "", err
	}
	// Always aliased. Postgres names a bare SUM() column "sum", MySQL names it
	// "SUM(total)", and a reader keyed on either breaks on the other.
	return fmt.Sprintf("SELECT %s(%s) AS value\nFROM (\n%s\n) AS %s%s",
		fn, field, inner, blockAlias, where(blk.Filter)), nil
}

// chartSQL buckets and folds.
func (b Builder) chartSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	fn, err := aggregateOf(ds, blk.Y)
	if err != nil {
		return "", err
	}
	y, err := column(ds, blk.Y.Field)
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
	/*
	   The expression again in GROUP BY, not an ordinal.

	   This was `GROUP BY 1` and the comment beside it said every dialect here
	   accepts the ordinal, which was true of the three that existed. SQL Server
	   does not: it reads the 1 as a constant and answers "each GROUP BY
	   expression must contain at least one column that is not an outer
	   reference", which is a sentence nobody would connect to this line.

	   The worry the ordinal avoided was having the bucket written in two places
	   that could drift apart. That does not apply when both are the same `x`:
	   there is one expression, interpolated twice. `ORDER BY 1` stays, because
	   an ordinal in ORDER BY is accepted everywhere including here.
	*/
	return fmt.Sprintf(
		"SELECT %s AS bucket, %s(%s) AS value\nFROM (\n%s\n) AS %s%s\nGROUP BY %s\nORDER BY 1",
		x, fn, y, inner, blockAlias, where(blk.Filter), x), nil
}

// tableSQL selects the named columns, ordered and capped.
func (b Builder) tableSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	cols := make([]string, 0, len(blk.Columns))
	for _, name := range blk.Columns {
		c, err := column(ds, name)
		if err != nil {
			return "", err
		}
		cols = append(cols, c)
	}
	order, err := orderBy(ds, blk.Sort)
	if err != nil {
		return "", err
	}
	// The limit is a literal, not an argument. It is an integer from a
	// definition rather than a caller, and several databases will not plan a
	// parameterised LIMIT as well as a constant one.
	return fmt.Sprintf("SELECT %s\nFROM (\n%s\n) AS %s%s%s\nLIMIT %d",
		strings.Join(cols, ", "), inner, blockAlias, where(blk.Filter), order, blk.Rows()), nil
}

func orderBy(ds definition.Dataset, keys []definition.SortKey) (string, error) {
	if len(keys) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		field, err := column(ds, k.Field)
		if err != nil {
			return "", err
		}
		dir := "ASC"
		if strings.EqualFold(k.Dir, "desc") {
			dir = "DESC"
		} else if k.Dir != "" && !strings.EqualFold(k.Dir, "asc") {
			return "", fmt.Errorf("%w: sort direction %q is not asc or desc", ErrBadTemplate, k.Dir)
		}
		parts = append(parts, field+" "+dir)
	}
	return "\nORDER BY " + strings.Join(parts, ", "), nil
}

// where renders a block's own filter.
//
// This is author-written SQL, at the same trust level as the dataset's query —
// an author who can write the whole statement can write a predicate. It is not
// caller input and never becomes one: a block filter is fixed by the
// definition, which is why it is not part of the Filters a request carries.
func where(filter string) string {
	if strings.TrimSpace(filter) == "" {
		return ""
	}
	return "\nWHERE " + filter
}
