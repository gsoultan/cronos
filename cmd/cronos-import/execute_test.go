package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/store/file"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
	_ "modernc.org/sqlite"
)

// TestImportedReportReturnsRows runs the imported definitions against a real
// database, and is the only test here that could have caught the hazard the
// importer warns about.
//
// Everything else proves the definitions are well formed: they validate, they
// encode, the repository loads them, the compiler turns each block into SQL.
// None of that notices that a field is named for a column the query does not
// return — a `.jrxml` full of `AS "CustomerName"` produces definitions that
// pass every one of those checks and fail on the first row read. So this one
// executes the SQL and counts what comes back.
//
// SQLite in memory, seeded from the demo warehouse, so it runs in `go test`
// with nothing installed and no environment.
func TestImportedReportReturnsRows(t *testing.T) {
	root := repoRoot(t)
	db := seeded(t, filepath.Join(root, "demo", "seed.sql"))

	out := filepath.Join(t.TempDir(), "definitions")
	im := newImporter(out, true)
	if err := im.walk([]string{filepath.Join(root, "examples", "jasper")}); err != nil {
		t.Fatal(err)
	}
	if im.blocked != 0 {
		t.Fatalf("the shipped example does not import cleanly: %d blocked", im.blocked)
	}

	repo, err := file.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	reports := repo.Reports()
	if len(reports) != 1 {
		t.Fatalf("imported %d reports, want 1", len(reports))
	}

	ds, err := repo.Dataset(context.Background(), reports[0].Dataset)
	if err != nil {
		t.Fatal(err)
	}
	b := query.NewBuilder(query.SQLite{}).WithClock(func() time.Time {
		return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	})
	// The whole demo period, so every seeded invoice is in range.
	args := map[string]any{"from_date": "2026-01-01", "to_date": "2026-12-31"}
	pr := principal.Principal{OrgID: "acme", ProjectID: "finance"}

	ran := 0
	for _, out := range reports[0].Outputs {
		for i, blk := range out.Layout {
			if blk.Kind == definition.TextBlock {
				continue
			}
			plan, _, err := b.BuildBlock(ds, blk, args, query.Filters{}, pr)
			if err != nil {
				t.Errorf("%s block %d (%s) will not compile: %v", out.Name, i, blk.Kind, err)
				continue
			}
			rows := count(t, db, plan)
			if rows == 0 {
				t.Errorf("%s block %d (%s) returned no rows against the seeded warehouse:\n%s",
					out.Name, i, blk.Kind, plan.SQL())
			}
			ran++
		}
	}
	if ran < 3 {
		t.Errorf("only %d blocks executed; the example is not exercising the import", ran)
	}
}

// count executes a compiled plan and returns how many rows it produced.
func count(t *testing.T, db *sql.DB, plan query.Plan) int {
	t.Helper()
	rows, err := db.Query(plan.SQL(), plan.Args()...)
	if err != nil {
		t.Errorf("executing:\n%s\n%v", plan.SQL(), err)
		return 0
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Errorf("reading rows: %v", err)
	}
	return n
}

// seeded returns an in-memory database holding the demo warehouse.
func seeded(t *testing.T, path string) *sql.DB {
	t.Helper()
	script, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("demo seed not present: %v", err)
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// One connection, because each connection to :memory: gets its own empty
	// database and the seed would then be invisible to the query.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(string(script)); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	return db
}

// repoRoot walks up to the module root, so the test does not depend on where it
// is run from.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not find the module root")
	return ""
}
