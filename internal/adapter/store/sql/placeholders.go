package sql

import "strings"

// sql rewrites the `?` placeholders these statements are written with into
// whatever the driver expects.
//
// The statements are written once, in the portable form, and the numbering is
// mechanical. Writing each query twice — once for Postgres and once for SQLite
// — is two places for a tenancy predicate to be dropped from one of them.
func (s *Store) sql(query string) string {
	if s.mark == nil {
		return query
	}
	var out strings.Builder
	out.Grow(len(query) + 16)

	n := 0
	for _, r := range query {
		if r != '?' {
			out.WriteRune(r)
			continue
		}
		n++
		out.WriteString(s.mark(n))
	}
	return out.String()
}
