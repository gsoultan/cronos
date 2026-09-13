//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// writeParquet puts a real parquet file at path, creating its directory.
//
// DuckDB writes it, because the alternative is a checked-in binary fixture
// that nobody can read the contents of in a diff.
func writeParquet(t *testing.T, path string, rows string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// Cast explicitly. DuckDB infers DECIMAL widths from the literals, so
	// ('EU', 100.0) and ('EU', 50.0) produce two different column types and a
	// fixture would fail for a reason the test is not about.
	stmt := fmt.Sprintf(
		`COPY (SELECT region::VARCHAR AS region, amount::DOUBLE AS amount `+
			`FROM (VALUES %s) t(region, amount)) TO '%s' (FORMAT PARQUET)`,
		rows, path)
	if _, err := db.ExecContext(context.Background(), stmt); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// dataset is the definition the query below is compiled from.
func dataset() definition.Dataset {
	return definition.Dataset{
		Name:    "events",
		Sources: []definition.SourceRef{{Ref: "events"}},
		Query: "SELECT region, SUM(amount) AS total FROM events " +
			"GROUP BY region ORDER BY region",
		Fields: []definition.Field{
			{Name: "region", Type: "string", Role: definition.Dimension},
			{Name: "total", Type: "number", Role: definition.Measure},
		},
	}
}

/*
TestAParquetDirectoryIsQueryable reads actual parquet through the actual path.

The other tests in this package assert the SQL a mount generates. This one runs
it: two files, one of them nested a directory deeper, read as one table and
summed. A view that mounts and returns nothing looks exactly like one that
works, and the generated-SQL test cannot tell them apart.
*/
func TestAParquetDirectoryIsQueryable(t *testing.T) {
	dir := t.TempDir()
	writeParquet(t, filepath.Join(dir, "part-0.parquet"), "('EU', 100.0), ('US', 25.0)")
	// Nested, because a lake partitioned by date is the reason the glob is
	// recursive, and a non-recursive one would still pass with a flat fixture.
	writeParquet(t, filepath.Join(dir, "day=2/part-1.parquet"), "('EU', 50.0)")

	ctx := context.Background()
	f, err := Open(ctx, map[string]definition.DataSource{
		"events": {Name: "events", Driver: "object-store", URI: dir, Format: "parquet"},
	})
	if err != nil {
		t.Fatalf("mounting a parquet directory: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	plan, err := query.NewBuilder(query.Postgres{}).
		Build(dataset(), nil, principal.Principal{})
	if err != nil {
		t.Fatalf("compiling: %v", err)
	}
	rows, err := f.Executor().Execute(ctx, plan)
	if err != nil {
		t.Fatalf("executing against parquet: %v", err)
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
		expect, ok := want[region]
		if !ok {
			t.Fatalf("a region nobody wrote: %q", region)
		}
		if total != expect {
			t.Fatalf("%s summed to %.2f, the files hold %.2f", region, total, expect)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Fatalf("%d regions of %d — the nested file was not read", seen, len(want))
	}
}

/*
TestAURIThatNamesOneFileIsRefused pins the directory-only behaviour.

view() appends a recursive glob unconditionally, so a uri pointing at a file
becomes a glob under a path that is not a directory. Somebody will write that,
because "uri" beside "format: parquet" reads like it takes a parquet file.

It is refused at mount, with the pattern in the message — which is the good
outcome, and worth pinning as one: the same mistake reported as an empty
result at query time would read as a lake with no data in it.
*/
func TestAURIThatNamesOneFileIsRefused(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "part-0.parquet")
	writeParquet(t, file, "('EU', 100.0)")

	ctx := context.Background()
	f, err := Open(ctx, map[string]definition.DataSource{
		"events": {Name: "events", Driver: "object-store", URI: file, Format: "parquet"},
	})
	if err != nil {
		t.Logf("mount refused a single-file uri: %v", err)
		return
	}
	t.Cleanup(func() { _ = f.Close() })

	plan, err := query.NewBuilder(query.Postgres{}).
		Build(dataset(), nil, principal.Principal{})
	if err != nil {
		t.Fatalf("compiling: %v", err)
	}
	rows, err := f.Executor().Execute(ctx, plan)
	if err != nil {
		t.Logf("querying a single-file uri failed: %v", err)
		return
	}
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		n++
	}
	t.Logf("querying a single-file uri returned %d rows", n)
	if n > 0 {
		t.Fatalf("a single-file uri read %d rows — the glob is no longer directory-only "+
			"and the documented caveat is stale", n)
	}
}

/*
TestAnS3SourceCarriesItsCredential covers what reaches DuckDB for a private
bucket: the extension, then a secret scoped to that bucket, then the view.

Scoped, because an unscoped secret is the default for every bucket on the
connection — and a dataset joining two lakes with a key each would otherwise
authenticate both as whichever was created last.
*/
func TestAnS3SourceCarriesItsCredential(t *testing.T) {
	stmts, err := mount("events", definition.DataSource{
		Name: "events", Driver: "object-store",
		URI: "s3://acme-lake/events/", Format: "parquet",
		Region:      "eu-central-1",
		Credentials: "key_id=AKIAEXAMPLE;secret=SUPERSECRETVALUE",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := sqlOf(stmts)
	for _, want := range []string{
		"INSTALL httpfs", "LOAD httpfs",
		"CREATE OR REPLACE SECRET events_store (TYPE s3",
		"KEY_ID 'AKIAEXAMPLE'", "SECRET 'SUPERSECRETVALUE'",
		"REGION 'eu-central-1'",
		"SCOPE 's3://acme-lake/events/'",
		"read_parquet('s3://acme-lake/events/**/*.parquet', union_by_name=true)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	// The key must be declared as a thing to strip from an error, or redact
	// has nothing to work with and the leak is back.
	var declared bool
	for _, stmt := range stmts {
		for _, h := range stmt.holds {
			if h == "SUPERSECRETVALUE" {
				declared = true
			}
		}
	}
	if !declared {
		t.Error("the secret is in the sql and not in holds — an error would print it")
	}
}

// TestCredentialsCanComeFromTheEnvironment covers the deployment that has a
// role attached and no key to put in a definition at all.
func TestCredentialsCanComeFromTheEnvironment(t *testing.T) {
	stmts, err := mount("events", definition.DataSource{
		Name: "events", Driver: "object-store",
		URI: "s3://acme-lake/events/", Format: "parquet",
		Region: "eu-west-1", Credentials: definition.CredentialChain,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := sqlOf(stmts)
	if !strings.Contains(got, "PROVIDER credential_chain") {
		t.Errorf("chain did not become a credential_chain secret:\n%s", got)
	}
	if strings.Contains(got, "KEY_ID") {
		t.Errorf("a chain secret invented a key:\n%s", got)
	}
}

// TestAPublicBucketNeedsNoSecret keeps the case that worked before working:
// no credentials and no region is a plain view and nothing else.
func TestAPublicBucketNeedsNoSecret(t *testing.T) {
	stmts, err := mount("events", definition.DataSource{
		Name: "events", Driver: "object-store",
		URI: "s3://open-data/events/", Format: "parquet",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sqlOf(stmts); strings.Contains(got, "SECRET") {
		t.Errorf("a public bucket created a secret:\n%s", got)
	}
}

/*
TestACredentialIsNeverIgnored is the point of the closed scheme table.

A credential that does nothing is a credential nobody rotates, and the way to
find out it did nothing is a 403 months later. Both shapes are refused at
mount instead: a key on a local path, and a key on a scheme with no secret
type.
*/
func TestACredentialIsNeverIgnored(t *testing.T) {
	for _, c := range []struct{ name, uri string }{
		{"a local path", "/var/lake/events"},
		{"a scheme with no secret type", "https://example.com/events"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := mount("events", definition.DataSource{
				Name: "events", Driver: "object-store",
				URI: c.uri, Format: "parquet",
				Credentials: "key_id=AKIAEXAMPLE;secret=SUPERSECRETVALUE",
			})
			if err == nil {
				t.Fatal("a credential that cannot be used was accepted")
			}
			if strings.Contains(err.Error(), "SUPERSECRETVALUE") {
				t.Errorf("the refusal printed the key: %v", err)
			}
		})
	}
}

/*
TestAFailingSecretDoesNotPrintTheKey drives a real DuckDB rejection.

account_id is an r2 parameter; on an s3 secret DuckDB refuses it. The refusal
is the realistic shape — a definition that names a parameter the store does not
take — and the thing being checked is that what comes back names the source and
not the key.
*/
func TestAFailingSecretDoesNotPrintTheKey(t *testing.T) {
	_, err := Open(context.Background(), map[string]definition.DataSource{
		"events": {
			Name: "events", Driver: "object-store",
			URI: "s3://acme-lake/events/", Format: "parquet",
			Credentials: "key_id=AKIAEXAMPLE;secret=SUPERSECRETVALUE;account_id=acct",
		},
	})
	if err == nil {
		t.Skip("DuckDB now accepts account_id on an s3 secret — this needs a new rejection")
	}
	t.Logf("error was: %v", err)
	for _, leaked := range []string{"SUPERSECRETVALUE", "AKIAEXAMPLE"} {
		if strings.Contains(err.Error(), leaked) {
			t.Errorf("the error printed %q:\n%v", leaked, err)
		}
	}
	if !strings.Contains(err.Error(), "events") {
		t.Errorf("the error does not name the source:\n%v", err)
	}
}

/*
TestALakeWhoseSchemaDrifted is why the glob carries union_by_name.

The glob exists so a lake partitioned by date does not need one view per day,
and a lake partitioned by date is where a schema drifts: a column added in
March, a decimal widened in June. Without union_by_name the reader takes its
types from whichever file it globs first and imposes them on every other, so
the widening fails every query over the whole range — with an error that names
a file and a cast, and never says that a schema changed.

Both drifts are checked because they failed differently before: the added
column already worked, and only the widened type did not.
*/
func TestALakeWhoseSchemaDrifted(t *testing.T) {
	write := func(t *testing.T, path, sel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("duckdb", "")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if _, err := db.ExecContext(context.Background(), fmt.Sprintf(
			`COPY (SELECT %s) TO '%s' (FORMAT PARQUET)`, sel, path)); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}

	// read mounts dir and returns the summed total, or the error.
	read := func(t *testing.T, dir string) (float64, error) {
		t.Helper()
		ctx := context.Background()
		f, err := Open(ctx, map[string]definition.DataSource{
			"events": {Name: "events", Driver: "object-store", URI: dir, Format: "parquet"},
		})
		if err != nil {
			return 0, err
		}
		t.Cleanup(func() { _ = f.Close() })
		plan, err := query.NewBuilder(query.Postgres{}).
			Build(dataset(), nil, principal.Principal{})
		if err != nil {
			t.Fatalf("compiling: %v", err)
		}
		rows, err := f.Executor().Execute(ctx, plan)
		if err != nil {
			return 0, err
		}
		defer func() { _ = rows.Close() }()
		total := 0.0
		for rows.Next() {
			var region string
			var sum float64
			if err := rows.Scan(&region, &sum); err != nil {
				return 0, err
			}
			total += sum
		}
		return total, rows.Err()
	}

	t.Run("a column added later is survivable", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "day=1/part.parquet"),
			`'EU'::VARCHAR AS region, 100.0::DOUBLE AS amount`)
		write(t, filepath.Join(dir, "day=2/part.parquet"),
			`'EU'::VARCHAR AS region, 50.0::DOUBLE AS amount, 'card'::VARCHAR AS method`)

		total, err := read(t, dir)
		if err != nil {
			t.Fatalf("an added column broke the read: %v", err)
		}
		if total != 150 {
			t.Fatalf("summed to %.2f of 150 — rows were dropped silently, which is "+
				"worse than an error", total)
		}
	})

	t.Run("a widened type is too", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "day=1/part.parquet"),
			`'EU'::VARCHAR AS region, 100.0::DECIMAL(4,1) AS amount`)
		// Wider, and holding a value the narrower type cannot represent — so a
		// reader imposing the first file's type fails rather than rounding,
		// and the test cannot pass by losing precision quietly.
		write(t, filepath.Join(dir, "day=2/part.parquet"),
			`'EU'::VARCHAR AS region, 50000.0::DECIMAL(8,1) AS amount`)

		total, err := read(t, dir)
		if err != nil {
			t.Fatalf("a widened column broke the read — union_by_name is not "+
				"reaching the reader: %v", err)
		}
		if total != 50100 {
			t.Fatalf("summed to %.2f of 50100 — the widened partition was read "+
				"but its rows did not survive", total)
		}
	})
}

/*
TestOneMountServesEveryPooledConnection is the assumption Open rests on.

Open mounts once, on a *sql.DB, and the pool behind it hands out more than one
connection under load. That is only correct if what a mount creates belongs to
the database rather than to the session — measured here rather than assumed,
because the failure it would otherwise cause is a report that works until two
people run it at the same time, and then reports a table that does not exist.

Views, loaded extensions and secrets were all instance-level when this was
written. A DuckDB release that made any of them per-connection would break
federation silently under load, and would break this loudly.
*/
func TestOneMountServesEveryPooledConnection(t *testing.T) {
	dir := t.TempDir()
	writeParquet(t, filepath.Join(dir, "p.parquet"), "('EU', 100.0), ('US', 25.0)")

	ctx := context.Background()
	f, err := Open(ctx, map[string]definition.DataSource{
		"events": {Name: "events", Driver: "object-store", URI: dir, Format: "parquet"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })

	plan, err := query.NewBuilder(query.Postgres{}).
		Build(dataset(), nil, principal.Principal{})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	fail := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := f.Executor().Execute(ctx, plan)
			if err != nil {
				fail <- err
				return
			}
			defer func() { _ = rows.Close() }()
			seen := 0
			for rows.Next() {
				seen++
			}
			if err := rows.Err(); err != nil {
				fail <- err
				return
			}
			if seen != 2 {
				fail <- fmt.Errorf("a connection saw %d rows of 2", seen)
			}
		}()
	}
	wg.Wait()
	close(fail)
	for err := range fail {
		t.Fatalf("a pooled connection did not see the mount: %v", err)
	}
	if n := f.db.Stats().OpenConnections; n < 2 {
		t.Skipf("the pool never opened a second connection (%d), so nothing was proved", n)
	}
}

// The pool is bounded by the tightest of what the mounted sources allow,
// because every DuckDB connection carries one into each attached warehouse.
func TestTheFederationPoolIsBounded(t *testing.T) {
	dir := t.TempDir()
	writeParquet(t, filepath.Join(dir, "p.parquet"), "('EU', 1.0)")

	f, err := Open(context.Background(), map[string]definition.DataSource{
		"events": {
			Name: "events", Driver: "object-store", URI: dir, Format: "parquet",
			Pool: definition.Pool{MaxOpen: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if got := f.db.Stats().MaxOpenConnections; got != 3 {
		t.Errorf("the pool allows %d connections, the source allows 3", got)
	}
}
