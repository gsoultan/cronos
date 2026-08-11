package query

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// Check reports whether a dataset's templates can ever compile.
//
// Called on save, alongside definition.Validate. Build catches all of this
// too, but Build runs when a schedule fires — which is to say at 6am, in a
// burst, with the author asleep and the only evidence a line in a delivery
// log. The same mistake is worth catching twice if the second time is while
// someone is looking at it.
func Check(ds definition.Dataset) error {
	declared := declaredNames(ds)

	refs, err := templateRefs(ds.Query)
	if err != nil {
		return err
	}
	for _, r := range refs {
		switch {
		case r.source == fromScope:
			return fmt.Errorf(
				"%w: dataset %q uses .scope.%s in its query — scope belongs in rowLevelSecurity, "+
					"where a missing value matches no rows instead of matching everything",
				ErrBadTemplate, ds.Name, r.name)
		case !declared[r.name]:
			return fmt.Errorf("%w: dataset %q uses .params.%s, which it does not declare",
				ErrBadTemplate, ds.Name, r.name)
		}
	}
	return checkPredicates(ds, declared)
}

func checkPredicates(ds definition.Dataset, declared map[string]bool) error {
	for i, s := range ds.RowLevelSecurity {
		refs, err := templateRefs(s.Predicate)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			// A predicate with no hole is the same text for everybody, which
			// means it is a filter the author wanted applied always — not row
			// scope. Saying so beats letting them believe it isolates anyone.
			return fmt.Errorf("%w: dataset %q row scope %d reads no .scope value, "+
				"so it restricts every caller identically", ErrBadTemplate, ds.Name, i)
		}
		for _, r := range refs {
			if r.source == fromParams && !declared[r.name] {
				return fmt.Errorf("%w: dataset %q row scope %d uses .params.%s, which it does not declare",
					ErrBadTemplate, ds.Name, i, r.name)
			}
		}
	}
	return nil
}

// CheckFilters reports whether a report's shared filters can bind.
//
// Every binding must name a field the dataset publishes. The field reaches SQL
// as text rather than as an argument — it is the only part of a filter
// predicate that does — so a typo here is a broken query at run time and a
// wrong column at best. Datasets are keyed by name.
func CheckFilters(filters []definition.Filter, datasets map[string]definition.Dataset) error {
	for _, f := range filters {
		if err := f.Validate(); err != nil {
			return err
		}
		for name, field := range f.Bind {
			ds, known := datasets[name]
			if !known {
				return fmt.Errorf("%w: filter %q binds to dataset %q, which this report does not read",
					ErrBadTemplate, f.Name, name)
			}
			if _, ok := ds.Field(field); !ok {
				return fmt.Errorf("%w: filter %q binds %s to %q, which is not a field of it",
					ErrBadTemplate, f.Name, name, field)
			}
		}
	}
	return nil
}

// ScopeKeys returns the .scope names a predicate reads.
//
// Exported so publish can compile a block against a principal that satisfies
// the dataset's row scope. Checking with an empty scope would exercise the
// FALSE substitution instead of the predicate, and a predicate that will not
// compile would pass review.
func ScopeKeys(predicate string) []string {
	refs, err := templateRefs(predicate)
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range refs {
		if r.source == fromScope {
			out = append(out, r.name)
		}
	}
	return out
}
