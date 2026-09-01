package registry

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/query"
)

// dialectFor maps a driver to the SQL it speaks.
//
// A closed table, because guessing produces statements that are subtly wrong
// rather than statements that fail: a placeholder style that happens to parse
// binds the wrong values to the right-looking query.
func dialectFor(driver string) (query.Dialect, error) {
	switch driver {
	case "postgres", "pgx", "duckdb":
		return query.Postgres{}, nil
	case "sqlite":
		return query.SQLite{}, nil
	case "mysql":
		return query.MySQL{}, nil
	case "sqlserver", "mssql":
		return query.SQLServer{}, nil
	}
	return nil, fmt.Errorf("registry: no dialect for driver %q", driver)
}

/*
sqlDriver maps a datasource's driver to the database/sql name registered by
whichever package the binary imported.

`postgres` is the name the format uses, the documentation uses and every
operator writing a definition will use. pgx registers itself as `pgx`, and
without this a datasource with `driver: postgres` fails to open with "unknown
driver (forgotten import?)" — which reads like a build problem and is a naming
one. The definition store has always done this mapping; datasources never did,
and nothing caught it because every fixture in the repository is SQLite.
*/
func sqlDriver(driver string) string {
	switch driver {
	case "postgres":
		return "pgx"
	case "mssql":
		// Both names are in use — "mssql" is what most people type and what
		// the older driver registered as; "sqlserver" is what this one does.
		// Accepting both and mapping here beats a definition that fails to
		// open with "unknown driver" for having used the wrong one of two
		// names for the same product.
		return "sqlserver"
	}
	return driver
}
