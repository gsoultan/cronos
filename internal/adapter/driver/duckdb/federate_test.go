//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

/*
Two engines, one query — which is the whole of what federation is for.

Everything beside this checks the SQL that mounts a source: that the alias is an
identifier, that the DSN is quoted rather than concatenated, that every
attachment is read only. Good checks, and none of them attaches anything. The
one that executes runs `SELECT count(*) FROM range(10)`, which proves the driver
is wired and proves nothing about federating.

So the feature that sells this — a report that reads a customer list from the
CRM's Postgres and invoices from somewhere else, in one query, without an ETL —
had never been run. This runs it: real Postgres, real SQLite, a join across
them, and the arithmetic checked.

It needs the network, once, because DuckDB downloads its postgres and sqlite
extensions on first use. That is the reason it lives behind an environment
variable rather than in the default suite, and the reason the ordinary tests
stay hermetic.
*/
func TestAJoinAcrossTwoEngines(t *testing.T) {
	dsn := os.Getenv("CRONOS_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CRONOS_POSTGRES_DSN — federating needs a second engine to federate with")
	}
	ctx := context.Background()

	// The customer list, in the CRM's database.
	pg, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pg.ExecContext(context.Background(), `DROP TABLE IF EXISTS cronos_fed_customers`)
		_ = pg.Close()
	})
	for _, q := range []string{
		`DROP TABLE IF EXISTS cronos_fed_customers`,
		`CREATE TABLE cronos_fed_customers (id TEXT PRIMARY KEY, name TEXT, region TEXT)`,
		`INSERT INTO cronos_fed_customers VALUES ('c-1','Acme','EU'),('c-2','Globex','US')`,
	} {
		if _, err := pg.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	// The invoices, somewhere else entirely. On disk, because DuckDB attaches a
	// file and an in-memory SQLite is not one.
	file := filepath.Join(t.TempDir(), "invoices.db")
	lite, err := sql.Open("sqlite", "file:"+file)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE invoices (id TEXT, customer_id TEXT, total REAL)`,
		`INSERT INTO invoices VALUES ('i-1','c-1',100.0),('i-2','c-1',50.0),('i-3','c-2',25.0)`,
	} {
		if _, err := lite.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := lite.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := Open(ctx, map[string]definition.DataSource{
		"crm":       {Name: "crm", Driver: "postgres", DSN: dsn},
		"invoicing": {Name: "invoicing", Driver: "sqlite", DSN: file},
	})
	if err != nil {
		t.Fatalf("mounting two sources: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	rows, err := f.db.QueryContext(ctx, `
		SELECT c.name, c.region, SUM(i.total) AS billed
		FROM crm.public.cronos_fed_customers c
		JOIN invoicing.invoices i ON i.customer_id = c.id
		GROUP BY c.name, c.region
		ORDER BY billed DESC`)
	if err != nil {
		t.Fatalf("the join across two engines failed: %v", err)
	}
	defer func() { _ = rows.Close() }()

	// Named rather than counted: a federation that returns the right number of
	// rows with the wrong sums is the failure worth catching, because it is the
	// one somebody would believe.
	want := map[string]struct {
		region string
		billed float64
	}{
		"Acme":   {"EU", 150},
		"Globex": {"US", 25},
	}
	seen := 0
	for rows.Next() {
		var name, region string
		var billed float64
		if err := rows.Scan(&name, &region, &billed); err != nil {
			t.Fatal(err)
		}
		expect, ok := want[name]
		if !ok {
			t.Fatalf("the join produced a customer nobody inserted: %q", name)
		}
		if region != expect.region || billed != expect.billed {
			t.Fatalf("%s: %s billed %.2f, and the two databases say %s billed %.2f",
				name, region, billed, expect.region, expect.billed)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Fatalf("the join produced %d customers of %d — an attachment that mounts and "+
			"returns nothing looks exactly like one that works", seen, len(want))
	}
}

/*
And the read-only attachment holds against the engine, not just in the string.

TestEveryAttachmentIsReadOnly checks that READ_ONLY appears in the SQL. This
checks that DuckDB honours it — cronos reads other people's production
databases, and a federation that can write to one is a different product with a
different risk.
*/
func TestAFederationCannotWriteToWhatItMounted(t *testing.T) {
	dsn := os.Getenv("CRONOS_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CRONOS_POSTGRES_DSN")
	}
	ctx := context.Background()

	pg, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pg.ExecContext(context.Background(), `DROP TABLE IF EXISTS cronos_fed_readonly`)
		_ = pg.Close()
	})
	for _, q := range []string{
		`DROP TABLE IF EXISTS cronos_fed_readonly`,
		`CREATE TABLE cronos_fed_readonly (id TEXT PRIMARY KEY)`,
		`INSERT INTO cronos_fed_readonly VALUES ('keep-me')`,
	} {
		if _, err := pg.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	f, err := Open(ctx, map[string]definition.DataSource{
		"crm": {Name: "crm", Driver: "postgres", DSN: dsn},
	})
	if err != nil {
		t.Fatalf("mounting: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if _, err := f.db.ExecContext(ctx,
		`DELETE FROM crm.public.cronos_fed_readonly`); err == nil {
		t.Fatal("a federation deleted from a database it mounted read-only")
	}

	var left int
	if err := pg.QueryRowContext(ctx,
		`SELECT count(*) FROM cronos_fed_readonly`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Fatalf("%d rows left of 1 — the refusal was reported and the row went anyway", left)
	}
}
