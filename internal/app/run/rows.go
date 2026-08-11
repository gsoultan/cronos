package run

// Rows is a result set being read.
//
// Deliberately the shape of *sql.Rows, so the standard library satisfies it
// without an adapter and nothing has to copy a result set to cross a boundary.
// It is an iterator rather than a slice because a table block's cap is the
// only thing bounding a query's output, and materialising first would make
// that cap a formality.
type Rows interface {
	Columns() ([]string, error)
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}
