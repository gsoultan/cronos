package query

import (
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// Filters are a report's shared filters and the values a caller sent for them.
type Filters struct {
	Defs   []definition.Filter
	Values map[string]FilterValue
}

// predicates compiles the filters that reach ds, and reports which did not.
//
// Order follows the report's declaration rather than the map, so two blocks on
// the same dataset produce the same statement and the same cache key.
func (b *binder) predicates(ds definition.Dataset, f Filters) ([]string, Coverage, error) {
	var (
		out []string
		cov Coverage
	)
	for _, def := range f.Defs {
		field, bound := def.Binds(ds.Name)
		if !bound {
			cov.Ignored = append(cov.Ignored, def.Name)
			continue
		}
		v, sent := f.Values[def.Name]
		if !sent {
			// Declared but left blank. The block *is* covered by this filter —
			// it just is not narrowing anything right now, and reporting it as
			// ignored would tell someone the block cannot be filtered at all.
			cov.Applied = append(cov.Applied, def.Name)
			continue
		}
		sql, err := b.filter(ds, def, field, v)
		if err != nil {
			return nil, Coverage{}, err
		}
		out = append(out, sql)
		cov.Applied = append(cov.Applied, def.Name)
	}
	return out, cov, nil
}

// filter compiles one filter into a predicate against ds.
func (b *binder) filter(ds definition.Dataset, def definition.Filter, field string, v FilterValue) (string, error) {
	if !v.Op.Valid() {
		return "", fmt.Errorf("%w: filter %q was sent operator %q, which is not one of them",
			ErrBadArgument, def.Name, v.Op)
	}
	// The field name is the one piece of a filter predicate that reaches SQL as
	// text rather than as an argument. It comes from the author's bind map, not
	// from the caller — but an author is not a reason to skip the check, so it
	// must name a real field of this dataset.
	if _, ok := ds.Field(field); !ok {
		return "", fmt.Errorf("%w: filter %q binds %s to %q, which is not a field of it",
			ErrBadTemplate, def.Name, ds.Name, field)
	}
	if err := checkArity(def, v); err != nil {
		return "", err
	}

	switch v.Op {
	case IsNull:
		return field + " IS NULL", nil
	case NotNull:
		return field + " IS NOT NULL", nil
	case Between:
		return fmt.Sprintf("%s BETWEEN %s AND %s",
			field, b.value(v.Values[0]), b.value(v.Values[1])), nil
	case In:
		return field + " IN (" + b.list(v.Values) + ")", nil
	case Contains:
		return b.contains(field, v.Values[0])
	}
	return field + " " + sqlOp[v.Op] + " " + b.value(v.Values[0]), nil
}

func checkArity(def definition.Filter, v FilterValue) error {
	want := arity[v.Op]
	if want == -1 {
		if len(v.Values) == 0 {
			return fmt.Errorf("%w: filter %q with %q was sent nothing to match",
				ErrBadArgument, def.Name, v.Op)
		}
		return nil
	}
	if len(v.Values) != want {
		return fmt.Errorf("%w: filter %q with %q takes %d value(s), got %d",
			ErrBadArgument, def.Name, v.Op, want, len(v.Values))
	}
	return nil
}

// value binds one argument and returns its placeholder.
func (b *binder) value(v any) string {
	b.args = append(b.args, v)
	return b.ph.At(len(b.args))
}

func (b *binder) list(vs []any) string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = b.value(v)
	}
	return strings.Join(out, ", ")
}
