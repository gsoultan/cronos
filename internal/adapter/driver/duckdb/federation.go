//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/platform/secret"

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
	bound(db, sources)

	for name, src := range sources {
		stmts, err := mount(name, src)
		if err != nil {
			db.Close()
			return nil, err
		}
		for _, stmt := range stmts {
			if _, err := db.ExecContext(ctx, stmt.sql); err != nil {
				db.Close()
				// The source is named and the statement is not. DuckDB prints
				// the line it could not parse, and for an ATTACH that line
				// holds a password and for a CREATE SECRET it holds a key.
				return nil, fmt.Errorf("duckdb: mounting %q (%s): %w",
					name, src.Driver, secret.Redact(err, stmt.holds...))
			}
		}
	}
	return &Federation{db: db}, nil
}

/*
bound limits the pool to the tightest of what the mounted sources allow.

DuckDB runs in this process and its own connections are cheap, which is the
reason this looked like it did not need bounding. What is not cheap is what
they are attached to: every connection DuckDB opens to a federation carries its
own connection to each warehouse mounted into it, so an unbounded pool here is
an unbounded connection count in somebody else's production database — the
thing the registry bounds each source for, arrived at the long way round.

The tightest rather than the sum, for the same reason the row and time limits
are: each of those numbers is an answer about one database, and a federation
reads several at once.
*/
func bound(db *sql.DB, sources map[string]definition.DataSource) {
	var open, idle int
	var idleFor, lifetime time.Duration
	first := true
	for _, src := range sources {
		p := src.Pool
		if first {
			open, idle, idleFor, lifetime = p.Open(), p.Idle(), p.IdleFor(), p.LifetimeOf()
			first = false
			continue
		}
		open = min(open, p.Open())
		idle = min(idle, p.Idle())
		idleFor = min(idleFor, p.IdleFor())
		lifetime = min(lifetime, p.LifetimeOf())
	}
	if first {
		// No sources is not reachable through the registry, which refuses a
		// dataset with none. Left correct rather than assumed.
		return
	}
	db.SetMaxOpenConns(open)
	db.SetMaxIdleConns(idle)
	db.SetConnMaxIdleTime(idleFor)
	db.SetConnMaxLifetime(lifetime)
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
