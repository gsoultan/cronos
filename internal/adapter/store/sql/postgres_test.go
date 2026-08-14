package sql_test

import (
	"context"
	"database/sql"
	"errors"
	driversql "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
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

/*
Publishing proves a definition against the database, not only against a dialect.

Compiling catches a field the dataset does not declare. It cannot catch a
column the warehouse does not have, a permission the connection lacks, or a
date grain over a column stored as text — which works on SQLite and MySQL,
because one is typeless and the other coerces, and is a type error on the
Postgres the dataset was written for.

A prepare rather than a run: it parses, resolves every name and resolves every
type, touches no rows and holds no lock. This is the check that turns "fails at
six in the morning in the middle of a burst" into "refused at publish".
*/
func TestPreparingCatchesWhatCompilingCannot(t *testing.T) {
	dsn := os.Getenv("CRONOS_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CRONOS_POSTGRES_DSN — SQLite is typeless and cannot show this")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		DROP TABLE IF EXISTS verify_textdates;
		CREATE TABLE verify_textdates (id TEXT, issued_at TEXT, total REAL)`); err != nil {
		t.Fatal(err)
	}

	executor := driversql.NewExecutor(db)
	builder := query.NewBuilder(query.Postgres{})

	ds := definition.Dataset{
		Name:    "textdates",
		Sources: []definition.SourceRef{{Ref: "warehouse"}},
		Query:   "SELECT id, issued_at, total FROM verify_textdates",
		Fields: []definition.Field{
			{Name: "id", Type: "string", Role: definition.Dimension},
			{Name: "issued_at", Type: "date", Role: definition.Dimension},
			{Name: "total", Type: "number", Role: definition.Measure, Aggregate: "sum"},
		},
	}
	block := definition.Block{
		Kind:  definition.ChartBlock,
		Chart: "bar",
		X:     definition.DimensionRef{Field: "issued_at", Grain: "month"},
		Y:     definition.MeasureRef{Field: "total", Aggregate: "sum"},
	}

	plan, _, err := builder.BuildBlock(ds, block, nil, query.Filters{}, member)
	if err != nil {
		t.Fatalf("it compiled fine, which is the point: %v", err)
	}

	err = executor.Verify(context.Background(), plan)
	if err == nil {
		t.Fatal("the database accepted a month grain over a text column")
	}
	if !strings.Contains(err.Error(), "date_trunc") {
		t.Fatalf("refused for a different reason: %v", err)
	}

	// And the cast the message tells an author to add makes it acceptable.
	ds.Query = "SELECT id, CAST(issued_at AS date) AS issued_at, total FROM verify_textdates"
	plan, _, err = builder.BuildBlock(ds, block, nil, query.Filters{}, member)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.Verify(context.Background(), plan); err != nil {
		t.Fatalf("the suggested fix does not work: %v", err)
	}
}

// A project member, which is who a publish check compiles as.
var member = principal.Principal{
	OrgID: "acme", ProjectID: "finance", ProjectRole: principal.ProjectEditor, Member: true,
}

/*
Four instances starting at once, which is what a rolling deploy is.

Every migration ran in its own transaction, which makes each one all-or-nothing
and says nothing about two processes doing it together. Against a fresh Postgres
this left three of four unable to start, and not at some exotic migration — at
the first statement:

	sql: recording migrations: ERROR: duplicate key value violates unique
	constraint "pg_type_typname_nsp_index"

because `CREATE TABLE IF NOT EXISTS` is not concurrency-safe in Postgres. Two
sessions both pass the existence check and one loses the race in the catalogue.

An orchestrator retries, so a deployment does converge — after a
CrashLoopBackOff on every deploy carrying a migration, which is exactly the kind
of noise that teaches people to ignore a restarting pod.

Only Postgres can show this. SQLite is one writer by construction, which is the
same reason this file exists at all.
*/
func TestFourInstancesCanStartAtOnce(t *testing.T) {
	dsn := os.Getenv("CRONOS_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CRONOS_POSTGRES_DSN to run against a real Postgres")
	}

	// Its own schema, so this cannot be the test that migrates the others'.
	name := "cronos_race_test"
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
	t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + name + ` CASCADE`) })

	scoped := dsn + "&options=-csearch_path%3D" + name

	const instances = 4
	var wg sync.WaitGroup
	failures := make([]error, instances)

	for i := range instances {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := sql.Open("pgx", scoped)
			if err != nil {
				failures[i] = err
				return
			}
			defer db.Close()
			failures[i] = store.New(db, store.Dollar).ForDriver("postgres").
				Migrate(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range failures {
		if err != nil {
			t.Errorf("instance %d could not start: %v", i, err)
		}
	}

	/*
	   And each migration ran exactly once.

	   The quieter half of the same failure: a lock that let two instances
	   through might not error at all — it would apply a migration twice and
	   leave two rows saying so, and only the ones whose DDL happens to be
	   non-idempotent would complain.
	*/
	db, err := sql.Open("pgx", scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var duplicates int
	if err := db.QueryRow(`
		SELECT count(*) FROM (
			SELECT id FROM cronos_schema_migrations GROUP BY id HAVING count(*) > 1
		) AS d`).Scan(&duplicates); err != nil {
		t.Fatal(err)
	}
	if duplicates != 0 {
		t.Fatalf("%d migrations were recorded more than once", duplicates)
	}
}

/*
Two instances want to schedule; one may.

This is the guarantee that lets every replica run with CRONOS_SCHEDULER=1. It
used to be a rule a deployment held in its head — set the flag twice and every
customer gets two statements, forget it and nobody gets one, and both are quiet
because the only party who notices is the recipient.

Only Postgres can arbitrate it, which is the whole reason this lives here.
*/
func TestOnlyOneInstanceLeads(t *testing.T) {
	dsn := os.Getenv("CRONOS_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CRONOS_POSTGRES_DSN — leadership needs a database that can arbitrate it")
	}

	// Two stores, as two processes would be: separate pools, separate sessions.
	open := func() *store.Store {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return store.New(db, store.Dollar).ForDriver("postgres")
	}

	name := "test:only-one-leads"
	first, second := open().Lease(name), open().Lease(name)
	t.Cleanup(func() { first.Release(); second.Release() })

	ctx := context.Background()
	if !first.Leading(ctx) {
		t.Fatal("nobody could take an uncontested claim")
	}
	if second.Leading(ctx) {
		t.Fatal("two instances both believe they are scheduling — every customer gets two")
	}

	// Leadership is kept, not re-won: asking again must not hand it to the
	// other one, or the two would alternate and each fire half the schedules.
	if !first.Leading(ctx) {
		t.Fatal("the leader lost its own claim by asking about it")
	}
	if second.Leading(ctx) {
		t.Fatal("the follower took a claim that was still held")
	}

	// And it hands over when released, which is what makes a rolling deploy
	// take one tick rather than a keepalive timeout.
	first.Release()
	if !second.Leading(ctx) {
		t.Fatal("nobody took over after the leader stood down")
	}
}

// Two names do not contend. A deployment serving three projects runs three
// schedulers, and one project's leader must not block another's.
func TestLeadershipIsPerName(t *testing.T) {
	dsn := os.Getenv("CRONOS_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CRONOS_POSTGRES_DSN")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := store.New(db, store.Dollar).ForDriver("postgres")

	acme, rival := s.Lease("test:acme/finance"), s.Lease("test:rival/finance")
	defer acme.Release()
	defer rival.Release()

	ctx := context.Background()
	if !acme.Leading(ctx) || !rival.Leading(ctx) {
		t.Fatal("one project's scheduler blocked another's")
	}
}

// A store that cannot arbitrate says so, rather than pretending. SQLite is one
// process by construction, so the honest answer is "you are the only one here".
func TestSqliteHasNoLeaseAndNeedsNone(t *testing.T) {
	s := open(t)
	if s.Lease("anything") != nil {
		t.Fatal("SQLite offered a lease it cannot enforce")
	}
	// nil is the "lead unconditionally" case, and it must be safe to use.
	var none *store.Lease
	if !none.Leading(context.Background()) {
		t.Fatal("a deployment with nothing to arbitrate refused to lead")
	}
	none.Release()
}
