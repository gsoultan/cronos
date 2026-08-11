package query

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// aggregates are the folds a block may ask for.
//
// A table, so the definition supplies a key and never a function name. It is
// the same reasoning as the operator list: this value comes from a file
// someone edits, and a lookup turns a typo into a message instead of into
// whatever the database does with an unknown identifier.
var aggregates = map[string]string{
	"sum": "SUM", "count": "COUNT", "avg": "AVG", "min": "MIN", "max": "MAX",
}

// aggregateOf resolves the fold for a measure reference, falling back to the
// aggregate the dataset's field declared.
//
// The field's aggregate is the default and not a law: one measure legitimately
// appears twice on a report as a sum and a count, and a block that says so
// should be believed.
func aggregateOf(ds definition.Dataset, m definition.MeasureRef) (string, error) {
	name := m.Aggregate
	if name == "" {
		f, ok := ds.Field(m.Field)
		if !ok {
			return "", fmt.Errorf("%w: %q is not a field of dataset %q",
				ErrBadTemplate, m.Field, ds.Name)
		}
		name = f.Aggregate
	}
	if name == "" {
		return "", fmt.Errorf("%w: %q says how to fold %q nowhere — not on the block, "+
			"not on the field", ErrBadTemplate, ds.Name, m.Field)
	}
	fn, ok := aggregates[name]
	if !ok {
		return "", fmt.Errorf("%w: %q is not an aggregate — use sum, count, avg, min or max",
			ErrBadTemplate, name)
	}
	return fn, nil
}

// column checks that a name the definition wants to write into SQL is a field
// the dataset publishes.
//
// Field names are the only part of a compiled block that is text rather than
// an argument. They come from a definition rather than a caller, but an author
// is not a reason to skip the check — and the dataset's field list is the
// public surface a report is allowed to bind to.
func column(ds definition.Dataset, name string) (string, error) {
	if _, ok := ds.Field(name); !ok {
		return "", fmt.Errorf("%w: %q is not a field of dataset %q", ErrBadTemplate, name, ds.Name)
	}
	return name, nil
}
