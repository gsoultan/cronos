package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

/*
Migrations are the ordered list of changes this store has ever made.

Create-if-absent got the first version off the ground and cannot do anything
else: it adds a table, and it cannot add a column, widen a type, backfill a
value or drop an index. The first time a release needs one of those against a
database that already holds a customer's definitions, there is no path that
does not involve somebody hand-writing SQL on production.

The rules, which are the whole of it:

  - An entry that has shipped is never edited. Its SQL has already run
    somewhere; changing it changes what a new deployment gets and not what an
    old one has, and the two silently diverge.
  - Ids are dense and ascending. A gap means an entry was deleted, which is
    the same divergence by another route.
  - Each runs in a transaction, so a failure halfway leaves the database where
    it started. Both databases this store supports have transactional DDL,
    which is what makes that true and is worth saying out loud because most do
    not.
  - Applying is idempotent by construction: what has run is recorded, and the
    recording happens in the same transaction as the change.
*/
type Migration struct {
	// ID orders them. Dense and ascending, from 1.
	ID int
	// Name is for the operator reading the table, not for the code.
	Name string
	// SQL may hold several statements separated by semicolons, and {{bytes}}
	// where a driver's binary type belongs.
	SQL string
}

// migrations is the list. Append only.
var migrations = []Migration{
	{
		ID:   1,
		Name: "initial schema",
		// The tables as they stood when create-if-absent was the whole story.
		// Still IF NOT EXISTS, which is what lets a database created by that
		// earlier code adopt this one without dropping anything: the first
		// migration runs, finds everything already there, and records itself.
		SQL: schema,
	},
}

// migrationTable records what has run. Created outside the ordered list,
// because it is what the ordered list is read against.
const migrationTable = `
CREATE TABLE IF NOT EXISTS cronos_schema_migrations (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  applied_at TEXT NOT NULL
)`

/*
Migrate brings the database up to the current schema.

Forward only. There is no down migration and there will not be one: rolling a
schema backwards on a database holding a customer's data is a decision made
with a backup and a maintenance window, not by a process that just started and
found a number it did not recognise.

A database ahead of this binary is refused rather than used. Two versions
running against one database is an ordinary state during a deploy, but a new
one that has already added a column and an old one that writes without it is
how a row goes missing a field nobody notices for a week.
*/
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, migrationTable); err != nil {
		return fmt.Errorf("sql: recording migrations: %w", err)
	}

	applied, err := s.applied(ctx)
	if err != nil {
		return err
	}
	if latest(applied) > last(migrations) {
		return fmt.Errorf(
			"sql: this database is at schema %d and this build only knows %d — "+
				"it was migrated by a newer cronos",
			latest(applied), last(migrations))
	}

	for _, m := range migrations {
		if applied[m.ID] {
			continue
		}
		if err := s.apply(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// applied reads which ids have run.
func (s *Store) applied(ctx context.Context) (map[int]bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM cronos_schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("sql: reading applied migrations: %w", err)
	}
	defer rows.Close()

	out := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// apply runs one migration and records it, both or neither.
func (s *Store) apply(ctx context.Context, m Migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // committed below; this is the failure path

	for _, stmt := range statements(Substitute(m.SQL, s.driver)) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sql: migration %d (%s): %w", m.ID, m.Name, err)
		}
	}

	// In the same transaction as the change it describes. Recorded separately
	// and a crash between the two leaves a migration that has run and does not
	// say so, which runs again on the next start.
	if _, err := tx.ExecContext(ctx, s.sql(
		"INSERT INTO cronos_schema_migrations (id, name, applied_at) VALUES (?, ?, ?)"),
		m.ID, m.Name, s.now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("sql: recording migration %d: %w", m.ID, err)
	}
	return tx.Commit()
}

// Forget drops the record of what has been applied, leaving the tables.
//
// Only a test wants this: it is the shape a database has when it was created
// by the version that had no migration table, and adopting one of those is the
// upgrade every existing deployment performs exactly once.
func (s *Store) Forget(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM cronos_schema_migrations")
	return err
}

// SchemaVersion is what the database is at, for a readiness check to report.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var at sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT MAX(id) FROM cronos_schema_migrations").Scan(&at)
	if err != nil {
		return 0, err
	}
	return int(at.Int64), nil
}

// Wanted is the schema version this build expects. Compared against
// SchemaVersion, it is the difference between "starting" and "started".
func Wanted() int { return last(migrations) }

// statements splits a migration into the commands it is made of.
//
// Comments first, then the split. A semicolon inside a comment is not a
// statement boundary, and splitting before stripping turns the rest of that
// comment into SQL — which fails with a syntax error naming a word from an
// English sentence and takes a while to recognise as such.
func statements(sqlText string) []string {
	var out []string
	for _, stmt := range strings.Split(stripComments(sqlText), ";") {
		if strings.TrimSpace(stmt) != "" {
			out = append(out, stmt)
		}
	}
	return out
}

func latest(applied map[int]bool) int {
	var at int
	for id := range applied {
		if id > at {
			at = id
		}
	}
	return at
}

func last(ms []Migration) int {
	if len(ms) == 0 {
		return 0
	}
	return ms[len(ms)-1].ID
}
