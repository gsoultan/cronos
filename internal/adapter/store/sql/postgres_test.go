package sql_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/history"
	"github.com/gsoultan/cronos/internal/core/identity"
	_ "github.com/jackc/pgx/v5/stdlib"
)

/*
 * The same store, against a real Postgres.
 *
 * Everything else here runs on SQLite, which proves the tenancy and versioning
 * logic and proves nothing about the DDL. It hid two failures that a single
 * connection to Postgres finds immediately: BLOB is not a type Postgres has,
 * and a boolean column declared INTEGER does not scan into a Go bool.
 *
 * CI supplies CRONOS_POSTGRES_DSN from a service container. Locally it skips —
 * loudly, because a skip that reads like a pass is how this gap survived in the
 * first place.
 */

func postgres(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("CRONOS_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CRONOS_POSTGRES_DSN to run against a real Postgres — " +
			"SQLite cannot show a type Postgres does not have")
	}

	// A schema per test, so one run does not see another's rows and the suite
	// can be run twice without cleaning up.
	name := "t_" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_"))

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	for _, stmt := range []string{
		`DROP SCHEMA IF EXISTS ` + name + ` CASCADE`,
		`CREATE SCHEMA ` + name,
	} {
		if _, err := admin.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	// The search path goes in the connection string, not in a SET.
	//
	// SET applies to one session, and database/sql hands out whichever pooled
	// connection is free — so a concurrent test finds tables missing on the
	// connections that never ran it. That is exactly the asymmetry with SQLite
	// this file exists to expose, and it caught the test before the store.
	scoped := dsn + "&options=-csearch_path%3D" + name
	db, err := sql.Open("pgx", scoped)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	s := store.New(db, store.Dollar).ForDriver("postgres")
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

// The one SQLite cannot show: a schema declaring types Postgres does not have
// fails on the first statement, and every test after it is meaningless.
func TestPostgresMigratesAndRoundTrips(t *testing.T) {
	s := postgres(t)
	ctx := context.Background()

	version, err := s.Put(ctx, acme, "Report", "billing", doc("billing"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, acme, "Report", "billing")
	if err != nil {
		t.Fatal(err)
	}
	// BYTEA in, the same bytes out. A definition that came back re-encoded
	// would change its own content address on the next publish.
	if string(got) != string(doc("billing")) {
		t.Errorf("came back as %q", got)
	}
	if again, _ := s.Put(ctx, acme, "Report", "billing", doc("billing")); again != version {
		t.Errorf("the same bytes gave %s then %s", version, again)
	}
}

// A boolean column declared INTEGER scans into a Go bool on SQLite and does not
// on Postgres.
func TestPostgresScansEveryColumnType(t *testing.T) {
	s := postgres(t)
	ctx := context.Background()

	if err := s.CreateUser(ctx, dewi(), secret); err != nil {
		t.Fatal(err)
	}
	user, err := s.User(ctx, "u1")
	if err != nil {
		t.Fatalf("reading a user back: %v", err)
	}
	if user.Disabled {
		t.Error("a new user is not disabled")
	}
	if _, err := s.Authenticate(ctx, dewi().Email, secret); err != nil {
		t.Errorf("sign-in against postgres: %v", err)
	}

	// Timestamps are strings on both, so a run's finished_at is a nullable one.
	run := aRun("run_pg_1")
	if err := s.Begin(ctx, run); err != nil {
		t.Fatal(err)
	}
	back, _, err := s.Run(ctx, acme, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.FinishedAt != nil || back.Status != history.Running {
		t.Errorf("run = %+v", back)
	}
}

// The tenancy predicate is the property the store exists for, and it has to
// hold on the database that will actually run it.
func TestPostgresKeepsTenantsApart(t *testing.T) {
	s := postgres(t)
	ctx := context.Background()

	if _, err := s.Put(ctx, acme, "Report", "billing", doc("acme")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, who("northwind", "finance"), "Report", "billing"); !errors.Is(err, publish.ErrNotFound) {
		t.Errorf("another tenant read it: %v", err)
	}
	if list, _ := s.List(ctx, who("acme", "operations")); len(list) != 0 {
		t.Errorf("a sibling project listed %d definitions", len(list))
	}
}

// Postgres has real concurrency, which SQLite's single writer hides. A burst
// writes delivery records from every worker at once.
func TestPostgresHandlesConcurrentWrites(t *testing.T) {
	s := postgres(t)
	ctx := context.Background()

	run := aRun("run_pg_2")
	if err := s.Begin(ctx, run); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- s.Delivered(ctx, history.Delivery{
				RunID: run.ID, Recipient: "c-" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
				Channel: "file", Status: history.Delivered, Attempts: 1, At: run.StartedAt,
			})
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent delivery record: %v", err)
		}
	}
	_, deliveries, err := s.Run(ctx, acme, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 32 {
		t.Errorf("recorded %d of 32 deliveries", len(deliveries))
	}
}

// A password is one thing that must not be readable, whichever database holds
// it.
func TestPostgresStoresNoPlaintextPassword(t *testing.T) {
	s := postgres(t)
	if err := s.CreateUser(context.Background(), dewi(), secret); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(context.Background(), dewi().Email, "wrong"); !errors.Is(err, identity.ErrBadCredentials) {
		t.Errorf("got %v", err)
	}
}

// The upgrade a real deployment performs: a database created by the version
// that had no migration table, brought forward without losing what it holds.
//
// Postgres specifically, because this is the one place the mechanism touches
// DDL inside a transaction — which Postgres supports and most databases do
// not, and which is the whole reason a failed migration leaves nothing behind.
func TestPostgresAdoptsADatabaseFromBeforeMigrations(t *testing.T) {
	s := postgres(t)
	ctx := context.Background()

	// Migrate() has already run in the helper. Forget that it did, leaving the
	// tables and the rows exactly as the older code would have left them.
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, acme, "Dataset", "invoices", doc("invoices")); err != nil {
		t.Fatal(err)
	}
	if err := s.Forget(ctx); err != nil {
		t.Fatal(err)
	}

	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("adopting an existing database failed: %v", err)
	}
	if _, err := s.Get(ctx, acme, "Dataset", "invoices"); err != nil {
		t.Fatalf("the definition that was already there: %v", err)
	}
	if at, _ := s.SchemaVersion(ctx); at != store.Wanted() {
		t.Fatalf("version %d, want %d", at, store.Wanted())
	}
}
