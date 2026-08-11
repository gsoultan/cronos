// Package query compiles a dataset, a caller's arguments and a principal into
// one statement that is safe to execute.
//
// # Nothing is assembled
//
// A caller's value never becomes SQL text. Every {{ .params.x }} hole in a
// dataset's query becomes a bind placeholder and the value travels beside the
// statement, so there is no quoting to get right and no escaping to forget.
// The compiler cannot be asked to substitute an identifier, a table name or a
// fragment: a report that needs a different shape of query is a different
// dataset.
//
// # Row scope is structural
//
// Only Build produces a Plan, and Build always applies the dataset's row
// scope. A Plan's SQL is unexported, so there is no path that runs a dataset's
// query without having gone through the wrapper — the guarantee is in the
// type, not in a reviewer remembering to check.
//
// When a scope value is missing the predicate becomes FALSE. See scope.go for
// why that rather than a NULL comparison, and docs/tenancy.md for why not an
// error.
package query
