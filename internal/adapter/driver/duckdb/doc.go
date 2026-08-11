// Package duckdb federates: one query reading a warehouse, a lake and a
// spreadsheet as though they were one database.
//
// # Behind a build tag
//
// DuckDB is a C++ library, so this needs cgo and adds several hundred
// megabytes to a module download. Neither cost belongs in a build that only
// ever reads one Postgres, so the whole package is `-tags duckdb`. Without it
// cronos is pure Go, cross-compiles to anything, and federation is a clear
// error rather than a missing symbol.
//
// # Read-only, always
//
// Every attachment is READ_ONLY. A reporting tool is given credentials to
// somebody's production warehouse, and the difference between a tool that
// cannot write and one that merely does not is the difference between a
// reviewable risk and an unreviewable one.
package duckdb
