package run

import "errors"

var (
	// ErrNotRenderable means the report has no output a caller can be shown.
	ErrNotRenderable = errors.New("run: report has no such output")
	// ErrPinned means the caller tried to change a parameter the report fixed.
	// Reported rather than ignored: a caller shown a report that does not match
	// what they asked for should be told why.
	ErrPinned = errors.New("run: parameter is pinned by the report")
	// ErrExecute wraps whatever the database said. The message reaches an
	// operator's log; what a caller sees is decided at the edge, because a
	// driver error names tables.
	ErrExecute = errors.New("run: execute")
)
