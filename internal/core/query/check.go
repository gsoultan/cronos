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
