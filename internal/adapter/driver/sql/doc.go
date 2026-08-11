// Package sql executes plans through database/sql.
//
// One adapter for every SQL source. The differences between engines are a
// driver name and a query.Dialect, both of which are configuration — so
// Postgres, MySQL, SQLite and DuckDB do not each need an implementation of
// the same twelve lines.
package sql
