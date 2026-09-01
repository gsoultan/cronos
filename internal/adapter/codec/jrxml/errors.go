package jrxml

import "errors"

// ErrParse marks a file this cannot read as JasperReports XML at all.
//
// Distinct from a finding: a finding is something the import decided not to
// carry, and this is the import not happening.
var ErrParse = errors.New("jrxml: cannot parse")

// ErrRefused marks a report whose meaning cannot be carried without changing it.
//
// Two cases, both in the package doc: a `$P!{}` parameter spliced into SQL as
// text, and a query in a language that is not SQL. Both would import to
// something that loads and is wrong, which is worse than not importing.
var ErrRefused = errors.New("jrxml: refused")
