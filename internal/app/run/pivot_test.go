package run_test

import (
	"reflect"
	"testing"

	"github.com/gsoultan/cronos/internal/app/run"
)

/*
A pivot, end to end against a real database: crossed where it is read, and sent
on as the table every renderer already draws.
*/

// pivotRows are a block's rows with the heading row on top, which is how a
// test can say what the table should look like in one literal.
func pivotRows(b run.Block) [][]string {
	head := make([]string, len(b.Columns))
	for i, c := range b.Columns {
		head[i] = c.Label
	}
	return append([][]string{head}, b.Rows...)
}

// Region down the side, carrier across the top. Scotland has no Baltic depot,
// and its cell is blank rather than a zero nobody sold.
func TestAPivotCrossesTwoDimensionsWithTotals(t *testing.T) {
	b := draw(t, `- kind: pivot
  title: Parcels by region and carrier
  value: {field: parcels}
  pivot:
    rows: {field: region}
    columns: {field: carrier}`)

	want := [][]string{
		{"Region", "Aurora", "Baltic", "Total"},
		{"England", "1,200", "300", "1,500"},
		{"Scotland", "700", "", "700"},
		{"Total", "1,900", "300", "2,200"},
	}
	if got := pivotRows(b); !reflect.DeepEqual(got, want) {
		t.Fatalf("pivot\n got %q\nwant %q", got, want)
	}
	if b.Total != 2 {
		t.Errorf("total %d, want England and Scotland — the totals line is not a row", b.Total)
	}
	if b.Kind != "table" {
		t.Errorf("kind %q, want table: a pivot reaches every renderer as the table it is", b.Kind)
	}
	for _, c := range b.Columns[1:] {
		if c.Align != "right" {
			t.Errorf("column %q is not right-aligned", c.Label)
		}
	}
	if out := marshal(t, b); out["kind"] != "table" || out["rows"] == nil {
		t.Errorf("payload %v", keys(out))
	}
}

// The total of a row of averages is the average of the rows behind it, not
// the mean of the cells: England's two depots average 750, Scotland's one 700,
// and all three 733.33 — where a mean of means would say 725.
func TestAPivotOfAveragesTotalsTheRowsNotTheCells(t *testing.T) {
	b := draw(t, `- kind: pivot
  title: Average parcels
  value: {field: parcels, aggregate: avg}
  pivot:
    rows: {field: region}
    columns: {field: carrier}`)

	want := [][]string{
		{"Region", "Aurora", "Baltic", "Total"},
		{"England", "1,200", "300", "750"},
		{"Scotland", "700", "", "700"},
		{"Total", "950", "300", "733.33"},
	}
	if got := pivotRows(b); !reflect.DeepEqual(got, want) {
		t.Fatalf("pivot\n got %q\nwant %q", got, want)
	}
}

// A minimum's total is the smallest cell, not their sum.
func TestAPivotOfMinimaTotalsTheSmallest(t *testing.T) {
	b := draw(t, `- kind: pivot
  title: Smallest depot
  value: {field: parcels, aggregate: min}
  pivot:
    rows: {field: region}
    columns: {field: carrier}`)
	last := b.Rows[len(b.Rows)-1]
	if want := []string{"Total", "700", "300", "300"}; !reflect.DeepEqual(last, want) {
		t.Fatalf("totals %q, want %q", last, want)
	}
}

// A date across the top is bucketed by its grain and labelled as one.
func TestAPivotBucketsADateAcrossTheTop(t *testing.T) {
	b := draw(t, `- kind: pivot
  title: Parcels by day
  value: {field: parcels}
  pivot:
    rows: {field: carrier}
    columns: {field: day, grain: day}`)
	want := []string{"Carrier", "1 Aug", "2 Aug", "Total"}
	if got := pivotRows(b)[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("headings %q, want %q", got, want)
	}
}

// A pivot that matched nothing is a table with no rows — and no total row
// claiming a total of nothing.
func TestAnEmptyPivotHasNoTotalRow(t *testing.T) {
	b := draw(t, `- kind: pivot
  title: Nothing
  filter: parcels < 0
  value: {field: parcels}
  pivot:
    rows: {field: region}
    columns: {field: carrier}`)
	if len(b.Rows) != 0 || len(b.Columns) != 2 {
		t.Fatalf("rows %q, columns %+v", b.Rows, b.Columns)
	}
}
