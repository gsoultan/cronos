package query

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	open  = "{{"
	close = "}}"
)

// holeRef is the only thing that may appear between the braces.
//
// Anchored, and narrow on purpose. Everything a template can express is
// something the compiler knows how to bind; there is no expression language
// here to grow an escape from.
var holeRef = regexp.MustCompile(`^\.(params|scope)\.([a-z][a-z0-9_]*)$`)

// parse splits a template into the literal text around its holes.
//
// Returns len(refs)+1 literals, so a caller can interleave them with
// placeholders and be sure it has not dropped one. That invariant is the
// reason for this shape: a parser that returned a token stream would let a
// caller emit the literals and forget a hole, and a forgotten hole in a
// predicate is a predicate that no longer restricts.
func parse(tmpl string) (lits []string, refs []ref, err error) {
	rest := tmpl
	for {
		i := strings.Index(rest, open)
		if i < 0 {
			return append(lits, rest), refs, nil
		}
		j := strings.Index(rest[i:], close)
		if j < 0 {
			return nil, nil, fmt.Errorf("%w: %q is never closed", ErrBadTemplate, open)
		}
		inner := strings.TrimSpace(rest[i+len(open) : i+j])
		r, err := parseRef(inner)
		if err != nil {
			return nil, nil, err
		}
		lits = append(lits, rest[:i])
		refs = append(refs, r)
		rest = rest[i+j+len(close):]
	}
}

func parseRef(inner string) (ref, error) {
	m := holeRef.FindStringSubmatch(inner)
	if m == nil {
		return ref{}, fmt.Errorf(
			"%w: %q is not .params.name or .scope.name", ErrBadTemplate, inner)
	}
	return ref{source: m[1], name: m[2]}, nil
}

// refs returns the holes a template uses, for validation that does not need
// the surrounding text.
func templateRefs(tmpl string) ([]ref, error) {
	_, r, err := parse(tmpl)
	return r, err
}
