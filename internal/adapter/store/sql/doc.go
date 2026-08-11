// Package sql stores definitions in a database, one row per definition and one
// per version.
//
// # What makes it multi-tenant
//
// Every statement is scoped by organization and project, taken from the
// principal and never from an argument. There is no method that reads a
// definition without them in the WHERE clause, and no way to pass one tenant
// while acting in another — the boundary is the shape of the SQL rather than a
// rule somebody remembers.
//
// # Portable SQL
//
// The statements use nothing outside ON CONFLICT and ordinary predicates, so
// the same store runs on Postgres and on SQLite. That is not a portability
// hobby: it is what lets the tenancy behaviour be tested in an ordinary `go
// test` rather than behind a container nobody runs locally.
package sql
