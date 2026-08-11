// Package registry opens a connection per datasource and routes a dataset to
// the right one.
//
// Until this existed a dataset's `sources:` was decorative — parsed,
// validated, checked for duplicate aliases, and then every query went to one
// configured database anyway. A dataset naming a warehouse now reaches that
// warehouse, with that warehouse's own statement timeout and row cap.
//
// # One source, or several
//
// One source is a direct connection: no engine in the middle, no extra copy of
// the rows, and the database's own planner doing the work. Several is
// federation, which needs DuckDB — and DuckDB needs cgo, so an untagged build
// says so rather than failing to find a driver.
package registry
