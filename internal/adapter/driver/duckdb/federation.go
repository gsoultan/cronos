//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"

	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/core/definition"

	_ "github.com/marcboeker/go-duckdb/v2"
)

// Federation is a DuckDB connection with a dataset's sources mounted.
type Federation struct {
	db *sql.DB
}

// Open mounts every source under the name the query uses for it.
//
// The mounts happen once, at open, and not per query: attaching a warehouse is
// a connection to somebody else's database, and doing it five thousand times
// during a burst is a burst their DBA notices.
func Open(ctx context.Context, sources map[string]definition.DataSource) (*Federation, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, err
	}

	for name, src := range sources {
		stmt, err := mount(name, src)
		if err != nil {
			db.Close()
			return nil, err
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			db.Close()
			// The statement is named but the DSN is not: it is in the text and
			// it holds a password.
			return nil, fmt.Errorf("duckdb: mounting %q (%s): %w", name, src.Driver, err)
		}
	}
	return &Federation{db: db}, nil
}

// Executor returns something that can run plans against the federation.
func (f *Federation) Executor() *sqldriver.Executor { return sqldriver.NewExecutor(f.db) }

// Close releases the connection and every attachment with it.
func (f *Federation) Close() error { return f.db.Close() }

// Mounts returns the names a dataset's query may reference, for a caller that
// wants to say so in an error rather than let the database do it.
func Mounts(ds definition.Dataset) []string {
	out := make([]string, 0, len(ds.Sources))
	for _, s := range ds.Sources {
		out = append(out, s.Name())
	}
	return out
}
