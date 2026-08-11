package query

// ref is one {{ .source.name }} hole in a template.
//
// Two sources and no more. A hole that named anything else would be a way to
// reach values the caller was never given — the parser rejects them rather
// than resolving to empty, because a silent empty in a predicate is a
// predicate that stopped restricting.
type ref struct {
	source string // "params" or "scope"
	name   string
}

const (
	fromParams = "params"
	fromScope  = "scope"
)
