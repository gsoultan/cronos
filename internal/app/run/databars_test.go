package run_test

import (
	"reflect"
	"testing"
)

// A data bar is each row's figure over the column's largest, and only in the
// columns that asked for one.
func TestATableDrawsDataBarsInTheColumnsItNames(t *testing.T) {
	b := draw(t, `- kind: table
  title: Depots
  columns: [depot, parcels, staff]
  bars: [parcels]
  sort: [{field: parcels, dir: desc}]`)

	if got, want := b.Columns[1].Bars, []float64{1, 0.5833, 0.25}; !reflect.DeepEqual(got, want) {
		t.Errorf("parcels bars %v, want %v", got, want)
	}
	if b.Columns[0].Bars != nil || b.Columns[2].Bars != nil {
		t.Errorf("bars in columns that did not ask: %+v", b.Columns)
	}
	if b.Rows[0][1] != "1,200" {
		t.Errorf("cell %q: a bar is drawn behind the figure, not instead of it", b.Rows[0][1])
	}
	cols := marshal(t, b)["columns"].([]any)
	if _, ok := cols[1].(map[string]any)["bars"]; !ok {
		t.Errorf("payload columns %v carry no bars", cols)
	}
}

// A sparkline is a column beside the row's name, each row against its own lowest
// and highest, and the total row's line is the totals'.
func TestAPivotDrawsASparklineAcrossItsDates(t *testing.T) {
	b := draw(t, `- kind: pivot
  title: Parcels by day
  value: {field: parcels}
  pivot:
    rows: {field: carrier}
    columns: {field: day, grain: day}
    sparkline: true`)

	want := [][]string{
		{"Carrier", "Trend", "1 Aug", "2 Aug", "Total"},
		{"Aurora", "", "1,900", "", "1,900"},
		{"Baltic", "", "", "300", "300"},
		{"Total", "", "1,900", "300", "2,200"},
	}
	if got := pivotRows(b); !reflect.DeepEqual(got, want) {
		t.Fatalf("pivot\n got %q\nwant %q", got, want)
	}
	spark := b.Columns[1].Spark
	if want := [][]float64{{1, 0}, {0, 1}, {1, 0}}; !reflect.DeepEqual(spark, want) {
		t.Errorf("spark %v, want %v — a day nothing matched is drawn at nothing", spark, want)
	}
}

// A pivot of one date has no line to draw, so it has no column for one.
func TestASparklineOfOnePointIsLeftOut(t *testing.T) {
	b := draw(t, `- kind: pivot
  title: Parcels on the day
  filter: depot <> 'Bristol'
  value: {field: parcels}
  pivot:
    rows: {field: carrier}
    columns: {field: day, grain: day}
    sparkline: true`)
	if got := pivotRows(b)[0]; !reflect.DeepEqual(got, []string{"Carrier", "1 Aug", "Total"}) {
		t.Errorf("headings %q", got)
	}
}
