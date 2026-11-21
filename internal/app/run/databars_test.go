package run_test

import (
	"reflect"
	"testing"

	"github.com/gsoultan/cronos/internal/app/run"
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

// A shaded pivot carries a ramp step for each cell, classed across all of
// them, and leaves the totals and the empty pairs plain.
func TestAShadedPivotStepsEveryCellButTheTotals(t *testing.T) {
	b := draw(t, `- kind: pivot
  title: Parcels
  value: {field: parcels}
  pivot:
    rows: {field: region}
    columns: {field: carrier}
    shade: true`)

	// England 1,200 and 300, Scotland 700 and nothing.
	aurora, baltic := b.Columns[1].Shades, b.Columns[2].Shades
	if len(aurora) != 3 || aurora[2] != -1 || baltic[1] != -1 {
		t.Fatalf("shades %v %v: a total and an empty pair are plain", aurora, baltic)
	}
	if !(aurora[0] > aurora[1] && aurora[1] > baltic[0]) {
		t.Errorf("shades %v %v: 1,200 should be darker than 700, darker than 300", aurora, baltic)
	}
	if b.Columns[0].Shades != nil || b.Columns[3].Shades != nil {
		t.Errorf("the labels or the totals column were shaded: %+v", b.Columns)
	}
}

// A stat against a fixed target says how far along it is, in the figure's
// own format; against a field, the target is folded over the same rows.
func TestAStatIsSetAgainstItsTarget(t *testing.T) {
	b := draw(t, `- kind: stat
  label: Parcels
  value: {field: parcels, aggregate: sum}
  target: {value: 2750, label: Plan}`)
	if b.Goal == nil || b.Goal.Label != "Plan" || b.Goal.Share != 0.8 || b.Goal.Said != "80%" || b.Goal.Value != "2,750" {
		t.Fatalf("goal %+v, want 2,200 as 80%% of a 2,750 plan", b.Goal)
	}
	if out := marshal(t, b); out["goal"] == nil {
		t.Errorf("payload %v carries no goal", keys(out))
	}

	b = draw(t, `- kind: stat
  label: Parcels per head
  value: {field: parcels, aggregate: sum}
  target: {field: staff, aggregate: sum}`)
	if b.Goal == nil || b.Goal.Value != "77" || b.Goal.Said != "2857%" {
		t.Fatalf("goal %+v, want 2,200 against 77 staff", b.Goal)
	}
}

// A faceted chart is its groups on one scale, and prints as a panel each.
func TestAFacetedChartPrintsAPanelASeries(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: column
  title: Parcels by region, a panel a carrier
  facet: true
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)
	if !b.Facets || len(b.Groups) != 2 || b.YAxis == nil {
		t.Fatalf("facets %v, groups %d, axis %v", b.Facets, len(b.Groups), b.YAxis)
	}
	if out := marshal(t, b); out["facets"] != true {
		t.Errorf("payload %v does not say facets", keys(out))
	}
	charts := run.Printable(run.View{Blocks: []run.Block{b}})
	if len(charts) != 1 || len(charts[0].Panels) != 2 {
		t.Fatalf("printed %+v, want one chart of two panels", charts)
	}
	if p := charts[0].Panels[1]; p.Title != "Baltic" || len(p.Marks) == 0 || p.Keys != nil {
		t.Errorf("panel %q: %d marks, keys %v", p.Title, len(p.Marks), p.Keys)
	}
}
