package query

import "errors"

// ErrBadArgument marks a caller's argument the compiler refuses.
//
// Distinct from a definition error: this one is an API response to whoever
// made the call, and it must never quote back anything that would help someone
// probe the shape of the query behind it.
var ErrBadArgument = errors.New("query: bad argument")

// ErrBadTemplate marks a query or predicate the compiler cannot parse. It is
// an authoring error, caught on save.
var ErrBadTemplate = errors.New("query: bad template")
