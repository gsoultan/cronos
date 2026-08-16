package run

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"
)

/*
What a spreadsheet export weighs.

Every other path in this product is bounded by something small: a table block
pages at the query level, a delivery holds one recipient. An export is the
exception — a workbook is written as a whole, so BlockRows returns the whole set
in a [][]any and holds it while the file is built.

That is fine and it is a number rather than a hope. AGENTS.md used to claim that
nothing materialises a full result set anywhere, which was true of two paths out
of three and would have been a surprise to whoever raised run.ExportLimit.

This measures the shape BlockRows produces rather than calling it, because the
cost is in the slices and the interface values and not in anything cronos does
with them. The bound is generous: the point is that a hundred thousand rows is
tens of megabytes and not hundreds, so a handful of concurrent exports is a
container sizing decision rather than an OOM.
*/
func TestAnExportIsTensOfMegabytesNotHundreds(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a hundred thousand rows")
	}

	const rows = ExportLimit
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `CREATE TABLE t (
		id TEXT, customer TEXT, region TEXT, amount REAL, issued TEXT, note TEXT)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	insert, err := tx.Prepare(`INSERT INTO t VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if _, err := insert.Exec(
			fmt.Sprintf("i-%d", i), fmt.Sprintf("c-%d", i%5000), "EU",
			float64(i)/7, "2026-01-15", "a line item note of ordinary length"); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	result, err := db.QueryContext(ctx,
		`SELECT id, customer, region, amount, issued, note FROM t`)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := result.Columns()
	if err != nil {
		t.Fatal(err)
	}

	// The same shape BlockRows builds.
	var out [][]any
	for result.Next() {
		cells := make([]any, len(cols))
		into := make([]any, len(cols))
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := result.Scan(into...); err != nil {
			t.Fatal(err)
		}
		out = append(out, cells)
	}
	if err := result.Err(); err != nil {
		t.Fatal(err)
	}
	_ = result.Close()

	runtime.ReadMemStats(&after)
	held := float64(after.HeapAlloc-before.HeapAlloc) / (1 << 20)

	if len(out) != rows {
		t.Fatalf("read %d rows of %d", len(out), rows)
	}
	// Measured at 32MB on this shape. A hundred is the line between "size the
	// container for it" and "this is the thing that killed the pod".
	if held > 100 {
		t.Fatalf("an export of %d rows x %d columns holds %.0f MB — AGENTS.md says tens",
			rows, len(cols), held)
	}
	t.Logf("%d rows x %d columns: %.1f MB", len(out), len(cols), held)

	runtime.KeepAlive(out)
}
