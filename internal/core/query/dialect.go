package query

// Dialect is what differs between databases once a statement is otherwise
// written.
//
// Three methods, and they are the only three the compiler has needed: how an
// argument is marked, how a date is truncated, and how rows are capped.
// Everything else about a compiled plan is portable SQL, and keeping this
// interface small is what stops it becoming a query builder with a database's
// worth of opinions.
type Dialect interface {
	// At returns the placeholder for argument n, counting from 1.
	At(n int) string
	// Bucket truncates a date expression to a grain — day, week, month,
	// quarter or year. It returns an error rather than approximating: a chart
	// bucketed by the wrong period is wrong in a way nobody reads as an error.
	Bucket(grain, expr string) (string, error)
	// Limit caps a statement at n rows, as the text to place directly after
	// SELECT and the text to place at the end.
	//
	// Two pieces because the databases disagree about where it goes, not only
	// about what it is called: LIMIT is a trailing clause and TOP is a prefix.
	// Returning a trailing "LIMIT n" for everyone is what this replaces, and
	// SQL Server answered it with "Incorrect syntax near 'LIMIT'" — a sentence
	// naming a keyword that is not in the dialect at all.
	Limit(n int) (prefix, suffix string)
}
