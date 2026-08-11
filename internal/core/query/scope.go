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

// wrap applies the dataset's row scope to an already-rendered query.
//
// A wrapper rather than an appended AND: the dataset's query is arbitrary SQL
// and may already have a WHERE, a GROUP BY, a UNION or a CTE. Only a subquery
// composes with all of them, and the predicate then reads against the fields
// the dataset publishes rather than against whatever tables happen to be
// underneath.
func (b *binder) wrap(inner string, scopes []definition.RowScope) (string, error) {
	if len(scopes) == 0 {
		return inner, nil
	}
	preds := make([]string, 0, len(scopes))
	for _, s := range scopes {
		p, err := b.predicate(s)
		if err != nil {
			return "", err
		}
		preds = append(preds, "("+p+")")
	}
	return "SELECT * FROM (\n" + trimStatement(inner) + "\n) AS " + subqueryAlias +
		"\nWHERE " + strings.Join(preds, " AND "), nil
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
