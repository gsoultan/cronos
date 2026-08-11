package definition

import "time"

// Limits bound what a single query may cost the source it runs against.
//
// Both are required in practice and defaulted here rather than left at zero,
// because a datasource with no statement timeout is a connection pool waiting
// to be held by a query nobody is watching, and one with no row cap is a
// report that worked on ten thousand rows and takes the server down at ten
// million.
type Limits struct {
	StatementTimeout Duration `json:"statementTimeout,omitempty" yaml:"statementTimeout,omitempty"`
	MaxRows          int      `json:"maxRows,omitempty" yaml:"maxRows,omitempty"`
}

// DefaultStatementTimeout and DefaultMaxRows apply when a source says nothing.
const (
	DefaultStatementTimeout = 30 * time.Second
	DefaultMaxRows          = 1_000_000
)

// Timeout is the timeout to actually use.
func (l Limits) Timeout() time.Duration {
	if l.StatementTimeout > 0 {
		return time.Duration(l.StatementTimeout)
	}
	return DefaultStatementTimeout
}

// Rows is the row cap to actually use.
func (l Limits) Rows() int {
	if l.MaxRows > 0 {
		return l.MaxRows
	}
	return DefaultMaxRows
}
