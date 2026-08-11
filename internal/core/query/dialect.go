package query

// Dialect is what differs between databases once a statement is otherwise
// written.
//
// Two methods, and they are the only two the compiler has needed: how an
// argument is marked, and how a date is truncated. Everything else about a
// compiled plan is portable SQL, and keeping this interface at two methods is
// what stops it becoming a query builder with a database's worth of opinions.
type Dialect interface {
	// At returns the placeholder for argument n, counting from 1.
	At(n int) string
	// Bucket truncates a date expression to a grain — day, week, month,
	// quarter or year. It returns an error rather than approximating: a chart
	// bucketed by the wrong period is wrong in a way nobody reads as an error.
	Bucket(grain, expr string) (string, error)
}
