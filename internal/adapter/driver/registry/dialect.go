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
	}
	return nil, fmt.Errorf("registry: no dialect for driver %q", driver)
}

// sqlDriver maps a datasource's driver to the database/sql name registered by
// whichever package the binary imported.
func sqlDriver(driver string) string {
	if driver == "pgx" {
		return "pgx"
	}
	return driver
}
