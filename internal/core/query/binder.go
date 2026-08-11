package query

import (
	"fmt"
	"strings"
)

// binder walks templates, emitting placeholders and collecting the values they
// stand for.
//
// It holds the argument list across every template in one plan — the dataset
// query and then each row-scope predicate — because placeholder numbering is
// per statement, not per template, and the wrapper's arguments follow the
// inner query's in the order the text puts them.
type binder struct {
	ph     Dialect
	params map[string]any
	scope  map[string]string
	// declared is the set of parameter names the dataset actually has. A hole
	// naming anything else would otherwise bind nil and quietly return the
	// wrong rows.
	declared map[string]bool
	// member exempts a project member from row scope. See scopePredicates.
	member bool
	args   []any
}

// render emits tmpl with each hole replaced by a placeholder.
//
// allowScope is false everywhere except a row-scope predicate. The fail-closed
// rule replaces a whole predicate when its scope is missing, and it can only do
// that for text it knows is a predicate — a .scope hole anywhere else would
// bind the empty string and run, which is the disclosure the rule exists to
// prevent.
func (b *binder) render(tmpl string, allowScope bool) (string, error) {
	lits, refs, err := parse(tmpl)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for i, lit := range lits {
		out.WriteString(lit)
		if i >= len(refs) {
			break
		}
		marker, err := b.bind(refs[i], allowScope)
		if err != nil {
			return "", err
		}
		out.WriteString(marker)
	}
	return out.String(), nil
}

// bind appends one argument and returns the placeholder standing for it.
func (b *binder) bind(r ref, allowScope bool) (string, error) {
	var v any
	switch r.source {
	case fromParams:
		if !b.declared[r.name] {
			return "", fmt.Errorf("%w: query uses .params.%s, which the dataset does not declare",
				ErrBadTemplate, r.name)
		}
		v = b.params[r.name]
	case fromScope:
		if !allowScope {
			return "", fmt.Errorf(
				"%w: .scope.%s is only available in rowLevelSecurity, not in the query",
				ErrBadTemplate, r.name)
		}
		// Absence is handled a whole predicate at a time — see predicate().
		// Reaching here with a missing key would bind the zero value, and a
		// comparison against that is a predicate that has stopped restricting.
		v = b.scope[r.name]
	}
	b.args = append(b.args, v)
	return b.ph.At(len(b.args)), nil
}
