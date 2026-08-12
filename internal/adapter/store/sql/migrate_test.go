package sql_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
)

// Create-if-absent could add a table and nothing else. These are the
// properties that let the next release change a column on a database that
// already holds a customer's definitions.

func TestMigratingTwiceChangesNothingTheSecondTime(t *testing.T) {
	s, db := fresh(t)

	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err := s.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("a second run failed: %v", err)
	}
	second, _ := s.SchemaVersion(context.Background())
	if first != second {
		t.Fatalf("the version moved from %d to %d without a new migration", first, second)
	}

	// One row per migration, not one per start.
	var rows int
	if err := db.QueryRow("SELECT COUNT(*) FROM cronos_schema_migrations").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != store.Wanted() {
		t.Fatalf("%d rows recorded for %d migrations", rows, store.Wanted())
	}
}

// A database created by the version that had no migration table at all. The
// first migration is still IF NOT EXISTS, so it finds everything there,
// changes nothing, and records itself.
func TestADatabaseFromBeforeMigrationsAdoptsThemWithoutLosingAnything(t *testing.T) {
	s, db := fresh(t)

	// Exactly what the old code did: the whole schema, no version recorded.
	// Comments stripped before the split, because a semicolon inside one is
	// not a statement boundary.
	for _, stmt := range strings.Split(stripComments(store.Schema("sqlite")), ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setting up the old shape: %v", err)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO cronos_definitions (org, project, kind, name, version, body, updated_at, updated_by)
		 VALUES ('acme','finance','Dataset','invoices','sha256:x','y','2026-08-01T00:00:00Z','dewi')`); err != nil {
		t.Fatalf("seeding a definition somebody already had: %v", err)
	}

	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("adopting an existing database failed: %v", err)
	}

	var name string
	if err := db.QueryRow(`SELECT name FROM cronos_definitions`).Scan(&name); err != nil {
		t.Fatalf("the definition that was already there: %v", err)
	}
	if name != "invoices" {
		t.Fatalf("got %q", name)
	}
	if at, _ := s.SchemaVersion(context.Background()); at != store.Wanted() {
		t.Fatalf("version %d, want %d", at, store.Wanted())
	}
}

// Two versions against one database is ordinary during a deploy. A new one
// that has added a column and an old one writing without it is how a row goes
// missing a field nobody notices for a week.
func TestADatabaseFromTheFutureIsRefused(t *testing.T) {
	s, db := fresh(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(
		`INSERT INTO cronos_schema_migrations (id, name, applied_at) VALUES (?, ?, ?)`,
		store.Wanted()+5, "from a newer cronos", "2027-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	err := s.Migrate(context.Background())
	if err == nil {
		t.Fatal("a database ahead of this build was accepted")
	}
	if !strings.Contains(err.Error(), "newer cronos") {
		t.Fatalf("the error does not say why: %v", err)
	}
}

// The list is append-only and dense. A gap means an entry was deleted, which
// makes a new deployment and an old one disagree about what has run.
func TestTheMigrationListIsDenseAndAscending(t *testing.T) {
	if store.Wanted() < 1 {
		t.Fatal("there are no migrations")
	}
	// Wanted() is the last id; dense means it equals the count.
	s, db := fresh(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM cronos_schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != store.Wanted() {
		t.Fatalf("%d migrations recorded but the last id is %d — the list has a gap",
			count, store.Wanted())
	}
}

var migrated int

func fresh(t *testing.T) (*store.Store, *sql.DB) {
	t.Helper()
	migrated++
	db, err := sql.Open("sqlite",
		fmt.Sprintf("file:migrate-%s-%d?mode=memory&cache=shared", t.Name(), migrated))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return store.New(db, store.Question), db
}

func stripComments(stmt string) string {
	var kept []string
	for _, line := range strings.Split(stmt, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
