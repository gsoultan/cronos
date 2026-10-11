package run

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

// pairs is a result set of (row, column, value) held in memory.
type pairs struct {
	at   int
	data [][3]any
}

func (p *pairs) Columns() ([]string, error) {
	return []string{"pivot_row", "pivot_column", "value"}, nil
}
func (p *pairs) Next() bool   { p.at++; return p.at <= len(p.data) }
func (p *pairs) Err() error   { return nil }
func (p *pairs) Close() error { return nil }
func (p *pairs) Scan(dest ...any) error {
	for i, v := range p.data[p.at-1] {
		*dest[i].(*any) = v
	}
	return nil
}

var summed = definition.Block{
	Kind: definition.PivotBlock, Value: definition.MeasureRef{Field: "n", Aggregate: "sum"},
	Pivot: &definition.Pivot{
		Rows:    definition.DimensionRef{Field: "r"},
		Columns: definition.DimensionRef{Field: "c", Grain: "month"},
	},
}

// Columns are put in date order, not in the order the first row met them nor
// in the order their labels sort: "Apr 2026" sorts before "Jan 2026" as text.
func TestAPivotPutsItsColumnsInDateOrder(t *testing.T) {
	jan, feb, apr := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	x, err := readPivot(summed, definition.Dataset{}, &pairs{data: [][3]any{
		{"a", apr, int64(1)}, {"b", jan, int64(2)}, {"b", feb, int64(3)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Jan 2026", "Feb 2026", "Apr 2026"}; !reflect.DeepEqual(x.columns, want) {
		t.Fatalf("columns %q, want %q", x.columns, want)
	}
}

// Past the cap, the rest of an axis is folded into its last column rather
// than dropped, so the cells across a row still add up to its total.
func TestAPivotFoldsTheColumnsPastItsCap(t *testing.T) {
	var data [][3]any
	for i := range PivotColumns + 5 {
		data = append(data, [3]any{"a", int64(i), int64(1)})
	}
	blk := summed
	blk.Pivot = &definition.Pivot{Rows: summed.Pivot.Rows, Columns: definition.DimensionRef{Field: "c"}}
	x, err := readPivot(blk, definition.Dataset{}, &pairs{data: data})
	if err != nil {
		t.Fatal(err)
	}
	if len(x.columns) != PivotColumns || x.columns[PivotColumns-1] != "6 more" {
		t.Fatalf("%d columns, last %q, want %d ending in \"6 more\"",
			len(x.columns), x.columns[len(x.columns)-1], PivotColumns)
	}
	if got := x.cells[0][PivotColumns-1].value; got != 6 {
		t.Errorf("the folded column holds %v, want the 6 it stands for", got)
	}
	if x.rowTotals[0].value != float64(PivotColumns+5) {
		t.Errorf("row total %v, want %d", x.rowTotals[0].value, PivotColumns+5)
	}
}

// A read that reaches the cap says its totals are of what it read.
func TestAPivotCutByItsCapSaysSo(t *testing.T) {
	data := make([][3]any, 0, query.PivotLimit+1)
	for i := range query.PivotLimit + 1 {
		data = append(data, [3]any{fmt.Sprint(i % 400), int64(i / 400), int64(1)})
	}
	x, err := readPivot(definition.Block{
		Kind: definition.PivotBlock, Value: summed.Value,
		Pivot: &definition.Pivot{Rows: summed.Pivot.Rows, Columns: definition.DimensionRef{Field: "c"}},
	}, definition.Dataset{}, &pairs{data: data})
	if err != nil {
		t.Fatal(err)
	}
	if !x.cut || x.total.value != query.PivotLimit {
		t.Fatalf("cut %v, total %v, want cut at %d", x.cut, x.total.value, query.PivotLimit)
	}
	if got := x.totalLabel(); got != "Total of the first 50,000 cells" {
		t.Errorf("total row says %q", got)
	}
}

// A measure that matched and was null prints as a dash; a pair nothing
// matched prints as nothing.
func TestAPivotTellsANullFromNothing(t *testing.T) {
	blk := summed
	blk.Pivot = &definition.Pivot{Rows: summed.Pivot.Rows, Columns: definition.DimensionRef{Field: "c"}}
	x, err := readPivot(blk, definition.Dataset{}, &pairs{data: [][3]any{
		{"a", "x", nil}, {"b", "y", int64(4)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, rows := x.table(blk, definition.Dataset{})
	if want := []string{"a", "—", "", ""}; !reflect.DeepEqual(rows[0], want) {
		t.Fatalf("row a %q, want %q", rows[0], want)
	}
}
