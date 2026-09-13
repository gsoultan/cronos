package duckdb

import "regexp"

// alias is the shape a mount name may take.
//
// The alias becomes a SQL identifier in an ATTACH, which is the one place a
// definition's text reaches a statement here.
var alias = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

/*
MountName reports whether a query may call a source this.

Untagged as well as tagged, because the caller that has to produce a good
message about it is the registry, and the registry is compiled either way. A
datasource name is a slug and may hold dashes; a mount name becomes an
identifier in a statement and may not, which is what `as:` on a source
reference is for.
*/
func MountName(s string) bool { return alias.MatchString(s) }
