package query

import (
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

func dated() definition.Dataset {
	ds := invoices()
	ds.Fields = append(ds.Fields,
		definition.Field{Name: "issued_at", Type: "date", Role: definition.Dimension},
		definition.Field{Name: "status", Type: "string", Role: definition.Dimension})
	return ds
}

func block(t *testing.T, ds definition.Dataset, blk definition.Block, d Dialect) string {
	t.Helper()
	plan, _, err := NewBuilder(d).WithClock(jan1).
		BuildBlock(ds, blk, map[string]any{"from": "2026-07-01"}, Filters{}, embedded("c-9"))
	if err != nil {
		t.Fatalf("build block: %v", err)
	}
	return plan.SQL()
}

// Reading four million rows to add them up is the difference between a report
// that opens and one that times out.
func TestAStatFoldsInTheDatabase(t *testing.T) {
	sql := block(t, dated(), definition.Block{
		Kind: definition.StatBlock, Label: "Total billed",
		Value: definition.MeasureRef{Field: "total", Aggregate: "sum"},
	}, Postgres{})

	if !strings.HasPrefix(sql, "SELECT SUM(total) AS value") {
		t.Errorf("the fold is not pushed down:\n%s", sql)
	}
	// Postgres names a bare SUM() column "sum" and MySQL names it "SUM(total)".
	if !strings.Contains(sql, "AS value") {
		t.Errorf("the aggregate is unaliased, so its column name is dialect-specific:\n%s", sql)
	}
}

// Row scope is inside the subquery, so a block cannot aggregate over rows the
// caller was never allowed to read.
func TestABlockCannotAggregatePastRowScope(t *testing.T) {
	sql := block(t, dated(), definition.Block{
		Kind:  definition.StatBlock,
		Value: definition.MeasureRef{Field: "total", Aggregate: "sum"},
	}, Postgres{})

	scope := strings.Index(sql, "customer_id = $")
	agg := strings.Index(sql, "SUM(total)")
	if scope < 0 {
		t.Fatalf("row scope is missing entirely:\n%s", sql)
	}
	if agg > scope {
		t.Errorf("the aggregate should wrap the scoped rows, not precede them:\n%s", sql)
	}
}

func TestABlockFallsBackToTheFieldsAggregate(t *testing.T) {
	sql := block(t, dated(), definition.Block{
		Kind:  definition.StatBlock,
		Value: definition.MeasureRef{Field: "total"}, // no aggregate on the block
	}, Postgres{})

	if !strings.Contains(sql, "SUM(total)") {
		t.Errorf("the dataset field declares sum; the block should inherit it:\n%s", sql)
	}
}

func TestChartsBucketPerDialect(t *testing.T) {
	blk := definition.Block{
		Kind: definition.ChartBlock, Chart: "bar", Title: "Billed by month",
		X: definition.DimensionRef{Field: "issued_at", Grain: "month"},
		Y: definition.MeasureRef{Field: "total", Aggregate: "sum"},
	}

	pg := block(t, dated(), blk, Postgres{})
	if !strings.Contains(pg, "date_trunc('month', issued_at) AS bucket") {
		t.Errorf("postgres should truncate natively:\n%s", pg)
	}
	lite := block(t, dated(), blk, SQLite{})
	if !strings.Contains(lite, "strftime('%Y-%m-01', issued_at) AS bucket") {
		t.Errorf("sqlite has no date_trunc:\n%s", lite)
	}
	// Repeating the expression in GROUP BY is a second place for it to differ.
	if !strings.Contains(pg, "GROUP BY 1") {
		t.Errorf("group by the ordinal:\n%s", pg)
	}
}

// A chart bucketed by the wrong period is wrong in a way nobody reads as an
// error, so a dialect that cannot do it says so.
func TestAnUnsupportedGrainIsRefusedNotApproximated(t *testing.T) {
	blk := definition.Block{
		Kind: definition.ChartBlock, Chart: "bar",
		X: definition.DimensionRef{Field: "issued_at", Grain: "quarter"},
		Y: definition.MeasureRef{Field: "total", Aggregate: "sum"},
	}
	if _, _, err := NewBuilder(SQLite{}).WithClock(jan1).
		BuildBlock(dated(), blk, map[string]any{"from": "2026-07-01"}, Filters{}, embedded("c-9")); err == nil {
		t.Error("sqlite cannot bucket by quarter compatibly and should say so")
	}
	// The same block compiles where the dialect can express it.
	if sql := block(t, dated(), blk, Postgres{}); !strings.Contains(sql, "date_trunc('quarter'") {
		t.Errorf("postgres can:\n%s", sql)
	}
}

func TestATableIsOrderedAndCapped(t *testing.T) {
	sql := block(t, dated(), definition.Block{
		Kind:     definition.TableBlock,
		Columns:  []string{"customer_name", "issued_at", "total"},
		Sort:     []definition.SortKey{{Field: "issued_at", Dir: "desc"}, {Field: "total"}},
		PageSize: 50,
	}, Postgres{})

	if !strings.HasPrefix(sql, "SELECT customer_name, issued_at, total") {
		t.Errorf("columns in declared order:\n%s", sql)
	}
	if !strings.Contains(sql, "ORDER BY issued_at DESC, total ASC") {
		t.Errorf("a tiebreak keeps two runs in the same order:\n%s", sql)
	}
	if !strings.HasSuffix(sql, "LIMIT 50") {
		t.Errorf("the page size should cap it:\n%s", sql)
	}
}

// A block with no limit is how a report that worked on ten thousand rows falls
// over on ten million.
func TestATableIsCappedEvenWhenTheAuthorForgot(t *testing.T) {
	sql := block(t, dated(), definition.Block{
		Kind: definition.TableBlock, Columns: []string{"total"},
	}, Postgres{})

	if !strings.HasSuffix(sql, "LIMIT 100") {
		t.Errorf("want the default cap:\n%s", sql)
	}
}

func TestBlocksAreRefused(t *testing.T) {
	cases := []struct {
		name string
		blk  definition.Block
		says string
	}{
		// Field names are the only part of a block that becomes text.
		{"a field the dataset does not publish", definition.Block{
			Kind: definition.StatBlock, Value: definition.MeasureRef{Field: "profit", Aggregate: "sum"},
		}, `"profit" is not a field`},

		{"an aggregate nobody implements", definition.Block{
			Kind: definition.StatBlock, Value: definition.MeasureRef{Field: "total", Aggregate: "median"},
		}, "not an aggregate"},

		{"a measure nothing says how to fold", definition.Block{
			Kind: definition.StatBlock, Value: definition.MeasureRef{Field: "customer_name"},
		}, "says how to fold"},

		{"a sort column that does not exist", definition.Block{
			Kind: definition.TableBlock, Columns: []string{"total"},
			Sort: []definition.SortKey{{Field: "sneaky"}},
		}, `"sneaky" is not a field`},

		{"a sort direction that is neither", definition.Block{
			Kind: definition.TableBlock, Columns: []string{"total"},
			Sort: []definition.SortKey{{Field: "total", Dir: "sideways"}},
		}, "not asc or desc"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := NewBuilder(Postgres{}).WithClock(jan1).BuildBlock(
				dated(), c.blk, map[string]any{"from": "2026-07-01"}, Filters{}, embedded("c-9"))
			if !errors.Is(err, ErrBadTemplate) {
				t.Fatalf("got %v, want ErrBadTemplate", err)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("message %q does not say %q", err, c.says)
			}
		})
	}
}
