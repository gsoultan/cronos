package query

import (
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// subqueryAlias names the wrapped dataset query. Postgres requires a subquery
// in FROM to be aliased, and a fixed name means a predicate author always
// knows what they are qualifying.
const subqueryAlias = "cronos_rows"

// noRows is what a predicate becomes when the scope it needs was not supplied.
//
// Literal FALSE, not a NULL comparison. `customer_id = NULL` is UNKNOWN and so
// happens to return nothing — but that is a property of the comparison the
// author chose, and the author chooses. Wrap the same hole in COALESCE, or
// write NOT IN, and the NULL stops being safe. FALSE is not a value in the
// predicate; it replaces the predicate, so nothing the author can write
// changes what it means.
const noRows = "FALSE"

// scopePredicates renders the dataset's row scope, one predicate per entry.
//
// A project member is exempt. Row scope exists to isolate an ISV's end
// customers from one another; a member of the project is protected by
// membership and by the project owning its resources, and applying it to them
// means an author cannot preview their own report — see
// principal.Principal.Member and docs/tenancy.md.
//
// Exemption comes from a signed audience rather than from an absent claim,
// which is the whole difference. "This token says it belongs to a project
// member" is a statement somebody made; "this token has no scope" is a
// statement nobody made, and reading the second as permission is how one
// missing claim becomes a full-table disclosure.
func (b *binder) scopePredicates(scopes []definition.RowScope) ([]string, error) {
	if b.member {
		return nil, nil
	}
	preds := make([]string, 0, len(scopes))
	for _, s := range scopes {
		p, err := b.predicate(s)
		if err != nil {
			return nil, err
		}
		preds = append(preds, p)
	}
	return preds, nil
}

// wrap narrows an already-rendered query by preds.
//
// A wrapper rather than an appended AND: the dataset's query is arbitrary SQL
// and may already have a WHERE, a GROUP BY, a UNION or a CTE. Only a subquery
// composes with all of them, and the predicates then read against the fields
// the dataset publishes rather than against whatever tables happen to be
// underneath.
//
// preds must be passed in the order they were rendered. Each rendering
// appended its arguments, so the text order and the placeholder order are the
// same list read twice.
func (b *binder) wrap(inner string, preds []string) string {
	if len(preds) == 0 {
		return inner
	}
	wrapped := make([]string, len(preds))
	for i, p := range preds {
		wrapped[i] = "(" + p + ")"
	}
	return "SELECT * FROM (\n" + trimStatement(inner) + "\n) AS " + subqueryAlias +
		"\nWHERE " + strings.Join(wrapped, " AND ")
}

// predicate renders one row-scope predicate, or FALSE if the scope it reads
// was not supplied.
//
// docs/tenancy.md: an absent scope value means the predicate matches nothing.
// Never dropped, and never read as "no constraint" — that reading turns one
// missing token claim into a full-table disclosure.
func (b *binder) predicate(s definition.RowScope) (string, error) {
	refs, err := templateRefs(s.Predicate)
	if err != nil {
		return "", err
	}
	for _, r := range refs {
		if r.source != fromScope {
			continue
		}
		if _, ok := b.scope[r.name]; !ok {
			return noRows, nil
		}
	}
	return b.render(s.Predicate, true)
}

// trimStatement removes the trailing semicolon an author naturally writes. It
// is a statement terminator, and inside a subquery it is a syntax error.
func trimStatement(sql string) string {
	return strings.TrimRight(strings.TrimSpace(sql), "; \t\n")
}
