//go:build duckdb

package registry_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/driver/registry"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"

	_ "github.com/marcboeker/go-duckdb/v2"
)

// lake writes a real parquet partition and returns the directory holding it.
func lake(t *testing.T, rows string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "events")
	if err := os.MkdirAll(filepath.Join(dir, "day=1"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(context.Background(), fmt.Sprintf(
		`COPY (SELECT region::VARCHAR AS region, amount::DOUBLE AS amount `+
			`FROM (VALUES %s) t(region, amount)) TO '%s' (FORMAT PARQUET)`,
		rows, filepath.Join(dir, "day=1/part.parquet"))); err != nil {
		t.Fatal(err)
	}
	return dir
}

func lakeDataset(name string) definition.Dataset {
	return definition.Dataset{
		Name:    name,
		Sources: []definition.SourceRef{{Ref: "events-lake", As: "events"}},
		Query: "SELECT region, SUM(amount) AS total FROM events " +
			"GROUP BY region ORDER BY region",
		Fields: []definition.Field{
			{Name: "region", Type: "string", Role: definition.Dimension},
			{Name: "total", Type: "number", Role: definition.Measure},
		},
	}
}

/*
TestADatasetReadsAParquetLake is the whole point of the federation package.

It was unreachable: nothing imported it, so a `driver: object-store` source
resolved to ErrNoFederation in every build — including one made with the tag
the message told the operator to rebuild with. The reader worked and the
product could not call it.

So this asserts the reachable thing rather than the reader: a datasource and a
dataset as an operator would write them, through the registry that a report
goes through, ending in rows.
*/
func TestADatasetReadsAParquetLake(t *testing.T) {
	dir := lake(t, "('EU', 100.0), ('US', 25.0), ('EU', 50.0)")

	reg, err := registry.New([]definition.DataSource{{
		Name: "events-lake", Driver: "object-store", URI: dir, Format: "parquet",
	}}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()
	if why := reg.Unavailable(); len(why) > 0 {
		t.Fatalf("the lake would not register: %v", why)
	}

	ctx := context.Background()
	engine, err := reg.Engine(ctx, lakeDataset("events"))
	if err != nil {
		t.Fatalf("a dataset reading a lake got no engine: %v", err)
	}

	plan, err := engine.Builder.Build(lakeDataset("events"), nil, principal.Principal{})
	if err != nil {
		t.Fatalf("compiling: %v", err)
	}
	rows, err := engine.Executor.Execute(ctx, plan)
	if err != nil {
		t.Fatalf("executing: %v", err)
	}
	defer func() { _ = rows.Close() }()

	want := map[string]float64{"EU": 150, "US": 25}
	seen := 0
	for rows.Next() {
		var region string
		var total float64
		if err := rows.Scan(&region, &total); err != nil {
			t.Fatal(err)
		}
		if total != want[region] {
			t.Errorf("%s summed to %.2f, the files hold %.2f", region, total, want[region])
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Fatalf("%d regions of %d — a mount that returns nothing looks like one "+
			"that works", seen, len(want))
	}
}

/*
TestTheFederationIsMountedOnceAndReused is why the registry keeps them.

duckdb.Open mounts everything at open and nothing per query, on the reasoning
that attaching somebody else's warehouse five thousand times during a burst is
a burst their DBA notices. That reasoning is worth nothing if the registry
opens a federation per report, which is what a cache-free implementation does.
*/
func TestTheFederationIsMountedOnceAndReused(t *testing.T) {
	dir := lake(t, "('EU', 1.0)")
	reg, err := registry.New([]definition.DataSource{{
		Name: "events-lake", Driver: "object-store", URI: dir, Format: "parquet",
	}}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	ctx := context.Background()
	first, err := reg.Engine(ctx, lakeDataset("events"))
	if err != nil {
		t.Fatal(err)
	}
	// A second dataset over the same sources, under the same names: the same
	// attachment answers both, so two reports are not two connections.
	second, err := reg.Engine(ctx, lakeDataset("events-again"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Executor == nil || second.Executor == nil {
		t.Fatal("an engine came back without an executor")
	}
	// The executors are separate wrappers by design — each carries the
	// dataset's own limits. What must be shared is the thing underneath them,
	// which is the attachment.
	if n := reg.Mounted(); n != 1 {
		t.Errorf("two datasets over one lake hold %d federations, want 1", n)
	}
}

// A dataset whose source is named with a dash cannot call it that in SQL, and
// the message says what to do rather than leaving DuckDB to report a name it
// could not parse.
func TestASourceNameAQueryCannotUseIsRefusedClearly(t *testing.T) {
	dir := lake(t, "('EU', 1.0)")
	reg, err := registry.New([]definition.DataSource{{
		Name: "events-lake", Driver: "object-store", URI: dir, Format: "parquet",
	}}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()

	ds := lakeDataset("events")
	ds.Sources = []definition.SourceRef{{Ref: "events-lake"}} // no `as:`

	_, err = reg.Engine(context.Background(), ds)
	if !errors.Is(err, registry.ErrBadMountName) {
		t.Fatalf("got %v, want ErrBadMountName", err)
	}
	if !strings.Contains(err.Error(), "as:") {
		t.Errorf("the message should say how to fix it: %v", err)
	}
}

// A federation opened after Close would attach somebody else's warehouse with
// nothing left to release it.
func TestAQueryAfterCloseDoesNotOpenAnything(t *testing.T) {
	dir := lake(t, "('EU', 1.0)")
	reg, err := registry.New([]definition.DataSource{{
		Name: "events-lake", Driver: "object-store", URI: dir, Format: "parquet",
	}}, nil, quiet())
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := reg.Engine(context.Background(), lakeDataset("events")); !errors.Is(err, registry.ErrClosed) {
		t.Fatalf("got %v, want ErrClosed", err)
	}
}
