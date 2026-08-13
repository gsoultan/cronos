package registry_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/driver/registry"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	_ "modernc.org/sqlite"
)

/*
 * Two datasets, two databases, routed by what they name.
 *
 * Until this package existed a dataset's `sources:` was parsed, validated and
 * ignored — every query went to one configured connection. A test using a
 * single database would still pass under that behaviour, so this one uses two
 * and asserts the rows came from the right one.
 */

func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// seed makes a file database holding one table with one telling row.
func seed(t *testing.T, name, marker string, rows int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE rows_ (id TEXT, total REAL)`); err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if _, err := db.Exec(`INSERT INTO rows_ VALUES (?, ?)`, marker, float64(i)); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func source(name, path string) definition.DataSource {
	return definition.DataSource{Name: name, Driver: "sqlite", DSN: path}
}

func dataset(name, sourceName string) definition.Dataset {
	return definition.Dataset{
		Name:    name,
		Sources: []definition.SourceRef{{Ref: sourceName}},
		Query:   "SELECT id, total FROM rows_",
		Fields: []definition.Field{
			{Name: "id", Type: "string", Role: definition.Dimension},
			{Name: "total", Type: "decimal", Role: definition.Measure, Aggregate: "sum"},
		},
	}
}

func member() principal.Principal {
	return principal.Principal{Subject: "u", OrgID: "o", ProjectID: "p",
		ProjectRole: principal.ProjectEditor, Member: true}
}

// The claim: a dataset naming a warehouse reaches that warehouse.
func TestEachDatasetReachesTheSourceItNames(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{
		source("warehouse", seed(t, "warehouse", "from-warehouse", 1)),
		source("archive", seed(t, "archive", "from-lake", 1)),
	}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	for _, c := range []struct{ dataset, from, want string }{
		{"invoices", "warehouse", "from-warehouse"},
		{"history", "archive", "from-lake"},
	} {
		t.Run(c.dataset, func(t *testing.T) {
			if got := readID(t, reg, dataset(c.dataset, c.from)); got != c.want {
				t.Errorf("read %q from %s, want %q", got, c.from, c.want)
			}
		})
	}
}

func readID(t *testing.T, reg *registry.Registry, ds definition.Dataset) string {
	t.Helper()
	ctx := context.Background()

	engine, err := reg.Engine(ctx, ds)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := engine.Builder.Build(ds, nil, member())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := engine.Executor.Execute(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("no rows")
	}
	var id string
	var total float64
	if err := rows.Scan(&id, &total); err != nil {
		t.Fatal(err)
	}
	return id
}

// A dataset naming a source nobody defined must say so, rather than falling
// back to whichever connection happened to be lying around.
func TestAnUnknownSourceIsRefused(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{
		source("warehouse", seed(t, "warehouse", "x", 1)),
	}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	_, err = reg.Engine(context.Background(), dataset("invoices", "nowhere"))
	if !errors.Is(err, registry.ErrUnknownSource) {
		t.Fatalf("got %v, want ErrUnknownSource", err)
	}
	if !strings.Contains(err.Error(), "nowhere") {
		t.Errorf("the message should name it: %v", err)
	}
}

// Two sources in one query is a join across databases. Named rather than
// attempted, so the message says what to build with.
func TestFederationSaysWhatItNeeds(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{
		source("warehouse", seed(t, "warehouse", "a", 1)),
		source("archive", seed(t, "archive", "b", 1)),
	}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	ds := dataset("joined", "warehouse")
	ds.Sources = append(ds.Sources, definition.SourceRef{Ref: "archive"})

	_, err = reg.Engine(context.Background(), ds)
	if !errors.Is(err, registry.ErrNoFederation) {
		t.Fatalf("got %v, want ErrNoFederation", err)
	}
	if !strings.Contains(err.Error(), "-tags duckdb") {
		t.Errorf("the message should say how to get it: %v", err)
	}
}

// An object store holds files rather than a catalogue, so reading one needs an
// engine that can address them even when it is the only source.
func TestAnObjectStoreAloneStillNeedsAnEngine(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{{
		Name: "lake", Driver: "object-store", URI: "s3://b/x", Format: "parquet",
	}}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	if _, err := reg.Engine(context.Background(), dataset("events", "lake")); !errors.Is(err, registry.ErrNoFederation) {
		t.Fatalf("got %v, want ErrNoFederation", err)
	}
}

// A server that starts with three of its four warehouses unreachable serves
// three-quarters of its reports and fails the rest at six in the morning.
func TestASourceThatWillNotOpenStopsStartup(t *testing.T) {
	_, err := registry.New([]definition.DataSource{
		{Name: "warehouse", Driver: "oracle", DSN: "x"},
	}, nil, quiet())
	if err == nil {
		t.Fatal("a driver nobody implements was accepted")
	}
	if !strings.Contains(err.Error(), "warehouse") {
		t.Errorf("the message should name the source: %v", err)
	}
}

// AGENTS.md: "Every datasource carries a statement timeout and a row cap. No
// unbounded query." The cap was defined and enforced nowhere.
func TestTheRowCapIsEnforced(t *testing.T) {
	src := source("warehouse", seed(t, "warehouse", "x", 50))
	src.Limits.MaxRows = 10

	reg, err := registry.New([]definition.DataSource{src}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	ctx := context.Background()
	ds := dataset("invoices", "warehouse")
	engine, err := reg.Engine(ctx, ds)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := engine.Builder.Build(ds, nil, member())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := engine.Executor.Execute(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	read := 0
	for rows.Next() {
		read++
	}
	// Refused, not truncated. A report that quietly stopped at the cap is a
	// wrong answer presented as a right one, and the reader cannot tell.
	if rows.Err() == nil {
		t.Fatalf("read %d rows past a cap of 10 without complaint", read)
	}
	if read > 10 {
		t.Errorf("read %d rows, cap was 10", read)
	}
	if !strings.Contains(rows.Err().Error(), "more than 10 rows") {
		t.Errorf("err = %v", rows.Err())
	}
}

/*
An in-memory database outlives an idle pool.

SQLite destroys a `mode=memory` database when the last connection to it closes,
and the pool this package configures is built to close connections: a few
minutes idle and every one of them is reaped. The next query opens a new, empty
database, and every report against that source fails with "no such table" —
while the source's Test button still passes, because connecting was never the
problem.

The demo warehouse is exactly such a source, so this is what somebody sees when
they open the demo, read a report, and come back after lunch. It was found
between two screenshots taken twenty minutes apart.

The test drives it deliberately rather than by waiting: an idle time of a
millisecond and a lifetime of a millisecond is the same reaping, arriving sooner.
*/
func TestAnInMemoryDatabaseSurvivesAnIdlePool(t *testing.T) {
	const dsn = "file:idle-pool-test?mode=memory&cache=shared"

	def := definition.DataSource{
		Name: "warehouse", Driver: "sqlite", DSN: dsn,
		Pool: definition.Pool{
			// As aggressive as the format allows. A real deployment says
			// minutes; the failure is identical and merely slower.
			MaxOpen: 2, MaxIdle: 1,
			MaxIdleTime: definition.Duration(time.Millisecond),
			MaxLifetime: definition.Duration(time.Millisecond),
		},
	}

	reg, err := registry.New([]definition.DataSource{def}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	db, ok := reg.DB("warehouse")
	if !ok {
		t.Fatal("the source did not open")
	}
	if _, err := db.Exec(`CREATE TABLE invoices (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO invoices VALUES (1)`); err != nil {
		t.Fatal(err)
	}

	/*
	   Long enough for the reaper to have run.

	   database/sql sweeps on a ticker whose interval is the smaller of the two
	   limits but never below a second, so a 50ms sleep proves nothing however
	   aggressive the configuration is — the sweep simply had not happened yet,
	   and the test passed for the wrong reason. The sibling test below catches
	   that: it asserts connections *were* closed, and it fails at 50ms.
	*/
	time.Sleep(1300 * time.Millisecond)

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM invoices`).Scan(&n); err != nil {
		t.Fatalf("the database was destroyed while nothing was using it: %v", err)
	}
	if n != 1 {
		t.Fatalf("the row is gone: %d", n)
	}
}

// And a source on disk keeps the limits it was given. The pinning above exists
// for one case, and a warehouse somebody else operates is not it: holding a
// connection open forever against their database is the behaviour their DBA
// turns this off for.
func TestAFileBackedSourceKeepsItsPoolLimits(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "warehouse.db")

	def := definition.DataSource{
		Name: "warehouse", Driver: "sqlite", DSN: dsn,
		Pool: definition.Pool{
			MaxOpen: 2, MaxIdle: 1,
			MaxIdleTime: definition.Duration(10 * time.Millisecond),
			MaxLifetime: definition.Duration(10 * time.Millisecond),
		},
	}

	reg, err := registry.New([]definition.DataSource{def}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	db, _ := reg.DB("warehouse")
	if _, err := db.Exec(`CREATE TABLE invoices (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1300 * time.Millisecond)

	// The connections were closed, which is the point — and the data is still
	// there, because it is on disk.
	if got := db.Stats().MaxIdleTimeClosed + db.Stats().MaxLifetimeClosed; got == 0 {
		t.Error("a file-backed source was pinned open along with the in-memory one")
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM invoices`).Scan(&n); err != nil {
		t.Fatalf("reopening a file-backed source: %v", err)
	}
}
