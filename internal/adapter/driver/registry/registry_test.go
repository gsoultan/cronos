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
	}, quiet())
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
	}, quiet())
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
	}, quiet())
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
	}}, quiet())
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
	}, quiet())
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

	reg, err := registry.New([]definition.DataSource{src}, quiet())
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
