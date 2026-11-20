package run_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
A field's format is how its figures are written everywhere they appear. It
was accepted and documented for months and applied nowhere, so a revenue
field marked format: currency still read "982,158.38" on every tile, bar,
axis and table — which is what these tests hold against.
*/

const formatSchema = `
CREATE TABLE orders (region TEXT, month TEXT, revenue REAL, met REAL, units REAL);
INSERT INTO orders VALUES
  ('North', '2026-01-01', 982158.38, 1, 3),
  ('North', '2026-02-01', 1250000.00, 0, 4),
  ('South', '2026-01-01', 412.50, 1, 2),
  ('South', '2026-02-01', 87000.00, 1, 5);`

const formatDataset = `
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: orders}
spec:
  sources: [{ref: warehouse}]
  query: SELECT region, month, revenue, met, units FROM orders
  fields:
    - {name: region,  type: string,  role: dimension}
    - {name: month,   type: date,    role: dimension}
    - {name: revenue, type: decimal, role: measure, aggregate: sum, format: currency, currency: EUR}
    - {name: met,     type: decimal, role: measure, aggregate: avg, format: percent}
    - {name: units,   type: number,  role: measure, aggregate: sum}
`

var formatDBs int

func drawFormatted(t *testing.T, layout string) []run.Block {
	t.Helper()
	formatDBs++
	db, err := sql.Open("sqlite", fmt.Sprintf("file:formats%d?mode=memory&cache=shared", formatDBs))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(formatSchema); err != nil {
		t.Fatal(err)
	}
	ds, err := yamlcodec.Loader{}.Dataset([]byte(formatDataset))
	if err != nil {
		t.Fatalf("dataset: %v", err)
	}
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: formats}
spec:
  dataset: orders
  outputs:
    - name: interactive
      renderer: interactive
      layout:
` + indent(layout)))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	s := run.New(datasets{"orders": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
	}})
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return v.Blocks
}

// A tile reads in the field's currency, compacted as every tile is.
func TestAStatIsWrittenInItsFieldsCurrency(t *testing.T) {
	b := drawFormatted(t, `- kind: stat
  label: Revenue
  value: {field: revenue}
- kind: stat
  label: Within SLA
  value: {field: met, aggregate: avg}
- kind: stat
  label: Units
  value: {field: units}`)
	for i, want := range []string{"€2.3M", "75%", "14"} {
		if b[i].Value != want {
			t.Errorf("tile %d reads %q, want %q", i, b[i].Value, want)
		}
	}
}

// A bar's figure drops cents once it is in the thousands, and keeps them
// below; the scale it is read against is in the same currency.
func TestAChartsFiguresAndScaleAreInTheCurrency(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: column
  x: {field: region}
  y: {field: revenue}`)[0]
	got := map[string]string{}
	for _, s := range b.Series {
		got[s.Label] = s.Formatted
	}
	if got["North"] != "€2.2M" || got["South"] != "€87,412" {
		t.Errorf("bars read %v, want €2.2M and €87,412", got)
	}
	last := b.YAxis.Ticks[len(b.YAxis.Ticks)-1]
	if !strings.HasPrefix(last.Label, "€") {
		t.Errorf("the scale is not in euros: %+v", b.YAxis.Ticks)
	}
}

// A table cell is exact: money keeps its cents and nothing is compacted.
func TestATablesCellsAreExactInTheirFormat(t *testing.T) {
	b := drawFormatted(t, `- kind: table
  columns: [region, revenue, met, units]
  sort: [{field: revenue, dir: desc}]`)[0]
	want := []string{"North", "€1,250,000.00", "0%", "4"}
	for i, w := range want {
		if b.Rows[0][i] != w {
			t.Errorf("column %d reads %q, want %q (row %v)", i, b.Rows[0][i], w, b.Rows[0])
		}
	}
}

// A share moves in points, not as a percentage of itself.
func TestAPercentTrendMovesInPoints(t *testing.T) {
	b := drawFormatted(t, `- kind: stat
  label: Within SLA
  value: {field: met, aggregate: avg}
  trend: {field: month, grain: month}`)[0]
	if b.Delta == nil || b.Delta.Value != "-50.0 pts" {
		t.Errorf("delta %+v, want -50.0 pts", b.Delta)
	}
	if b.Trend[0].Formatted != "100%" {
		t.Errorf("trend reads %+v", b.Trend)
	}
}

// A combo writes each measure in its own format, and each scale in the
// format of what is read against it.
func TestAComboWritesEachMeasureItsOwnWay(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: combo
  x: {field: month, grain: month}
  metrics:
    - {field: revenue, draw: bar}
    - {field: met, aggregate: avg, draw: line, axis: secondary}`)[0]
	if b.Tracks[0].Bars[0].Formatted != "€982,571" || b.Tracks[1].Bars[0].Formatted != "100%" {
		t.Errorf("tracks read %q and %q", b.Tracks[0].Bars[0].Formatted, b.Tracks[1].Bars[0].Formatted)
	}
	if b.Axis2 == nil || !strings.HasSuffix(b.Axis2.Ticks[len(b.Axis2.Ticks)-1].Label, "%") {
		t.Errorf("second scale %+v, want percentages", b.Axis2)
	}
}

// A field that asks for nothing reads exactly as it always has.
func TestAPlainFieldIsUnchanged(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: bar
  x: {field: region}
  y: {field: units}`)[0]
	if b.Series[0].Formatted != "7" && b.Series[0].Formatted != "7.00" {
		t.Errorf("a plain field was rewritten: %+v", b.Series)
	}
}
