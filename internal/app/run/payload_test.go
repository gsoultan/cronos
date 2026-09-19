package run_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/render/paginated"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/app/run"
)

/*
 * The payload every chart type actually produces.
 *
 * Written to disk for the embed package's browser check to serve, because the
 * two ends were being tested against two different fictions: the Go tests
 * assert what this package builds, and the viewer's tests assert what its own
 * hand-written stubs contain. Both pass while a renamed field leaves the real
 * product blank, and nothing in either suite is in a position to notice.
 *
 * Regenerate with `go test ./internal/app/run/ -run Payload`.
 */

// payloadReport draws one block of every chart type over the depots dataset.
const payloadReport = `
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: every-chart, title: Every chart}
spec:
  dataset: depots
  filters:
    - {name: carrier, label: Carrier, type: enum, values: [Aurora, Baltic], bind: {depots: carrier}}
    - {name: parcels_over, label: Parcels over, type: number, bind: {depots: parcels}}
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - {kind: stat, label: Parcels, value: {field: parcels, aggregate: sum}}
        - kind: chart
          chart: bar
          title: Parcels by region
          x: {field: region}
          y: {field: parcels, aggregate: sum}
        - kind: chart
          chart: bar
          title: Stacked by carrier
          stacked: true
          x: {field: region}
          series: {field: carrier}
          y: {field: parcels, aggregate: sum}
        - kind: chart
          chart: line
          title: Line
          x: {field: region}
          y: {field: parcels, aggregate: sum}
        - kind: chart
          chart: area
          title: Area
          x: {field: region}
          y: {field: parcels, aggregate: sum}
        - kind: chart
          chart: pie
          title: Pie
          x: {field: region}
          y: {field: parcels, aggregate: sum}
        - kind: chart
          chart: donut
          title: Donut
          x: {field: region}
          y: {field: parcels, aggregate: sum}
        - kind: chart
          chart: scatter
          title: Scatter
          x: {field: region}
          xValue: {field: staff, aggregate: sum}
          y: {field: parcels, aggregate: sum}
        - kind: chart
          chart: bubble
          title: Bubble
          x: {field: region}
          xValue: {field: staff, aggregate: sum}
          y: {field: parcels, aggregate: sum}
          size: {field: staff, aggregate: sum}
        - kind: chart
          chart: map
          title: Map
          x: {field: region}
          y: {field: parcels, aggregate: sum}
          map:
            layers: [polygon, heat, bubble, flow]
            geometry: shape
            lat: lat
            lon: lon
            toLat: to_lat
            toLon: to_lon
        - kind: chart
          chart: combo
          title: Combo
          x: {field: region}
          metrics:
            - {field: parcels, aggregate: sum, label: Parcels, draw: bar}
            - {field: staff, aggregate: sum, label: Staff, draw: line, axis: secondary}
        - kind: chart
          chart: funnel
          title: Funnel
          metrics:
            - {field: parcels, aggregate: sum, label: Handled}
            - {field: staff, aggregate: sum, label: Delivered}
        - kind: chart
          chart: waterfall
          title: Waterfall
          x: {field: region}
          y: {field: parcels, aggregate: sum}
        - kind: chart
          chart: heatmap
          title: Heatmap
          x: {field: region}
          series: {field: carrier}
          y: {field: parcels, aggregate: sum}
        - kind: chart
          chart: gauge
          title: Gauge
          y: {field: parcels, aggregate: sum}
          target: {value: 1500, label: Plan}
        - kind: chart
          chart: treemap
          title: Treemap
          x: {field: carrier}
          series: {field: region}
          y: {field: parcels, aggregate: sum}
        - kind: table
          title: Depots
          columns: [region, carrier, parcels]
`

// payloadPath is where the viewer's browser check reads it back.
var payloadPath = filepath.Join("..", "..", "..", "packages", "embed", "testdata", "payload.json")

func TestPayloadForEveryChartType(t *testing.T) {
	s, _ := geoReport(t, `- kind: text
  text: placeholder`)

	rep, err := yamlcodec.Loader{}.Report([]byte(payloadReport))
	if err != nil {
		t.Fatalf("report: %v", err)
	}

	view, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// Every chart type, one block each, plus a stat and a table.
	if len(view.Blocks) != 17 {
		t.Fatalf("want a block per chart type, got %d", len(view.Blocks))
	}
	if len(view.Filters) != 2 {
		t.Fatalf("the filter bar lost its controls: %+v", view.Filters)
	}

	raw, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Every chart carries the key its own type owns. Checked here rather than
	// only in the viewer, because a payload that is missing one renders as an
	// empty panel and an empty panel looks like a report that matched no rows.
	for _, want := range []string{
		`"chart": "bar"`, `"chart": "line"`, `"chart": "area"`,
		`"chart": "pie"`, `"chart": "donut"`, `"chart": "scatter"`,
		`"chart": "bubble"`, `"chart": "map"`, `"chart": "combo"`,
		`"chart": "funnel"`, `"chart": "waterfall"`, `"chart": "heatmap"`,
		`"chart": "gauge"`, `"chart": "treemap"`,
		`"groups"`, `"totals"`, `"points"`, `"tracks"`, `"stages"`,
		`"steps"`, `"cells"`, `"rects"`, `"gauge"`, `"shapes"`, `"markers"`, `"arcs"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the payload has no %s", want)
		}
	}

	if err := os.MkdirAll(filepath.Dir(payloadPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payloadPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
}

// TestEveryChartTypeReachesAPDF renders the same report to paper.
//
// The two halves were each proving their own half: internal/app/run asserts
// what it builds, and the paginated package asserts that marks typeset. This
// is the one that fails if the mapping between them is wrong — real database,
// real render, real typesetter, one PDF.
//
// Charts reached a PDF as nothing at all until this existed: the layout was
// walked for a table and every chart block was skipped, and the format
// documentation said they came out as static images.
func TestEveryChartTypeReachesAPDF(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst is not installed; the mapping is still covered by TestPayloadForEveryChartType")
	}

	s, _ := geoReport(t, `- kind: text
  text: placeholder`)

	// The same layout as the interactive payload, on a paginated output.
	printed := strings.Replace(payloadReport,
		"      renderer: interactive", "      renderer: paginated\n      page: {size: a4, orientation: landscape}", 1)
	printed = strings.Replace(printed, "name: interactive", "name: pdf", 1)
	printed = strings.Replace(printed, "name: every-chart", "name: every-chart-pdf", 1)

	rep, err := yamlcodec.Loader{}.Report([]byte(printed))
	if err != nil {
		t.Fatalf("report: %v", err)
	}

	statements := run.NewStatements(s, paginated.New(paginated.TypstCLI{}))
	res, err := statements.Statement(context.Background(), rep, "pdf", nil, anyone())
	if err != nil {
		t.Fatalf("statement: %v", err)
	}

	if !bytes.HasPrefix(res.Document, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF: %q", res.Document[:min(8, len(res.Document))])
	}
	// Fourteen charts of vector marks is a materially larger document than the
	// table alone; a PDF this size is one the marks never reached.
	if len(res.Document) < 8000 {
		t.Errorf("the PDF is %d bytes — too small to carry fourteen charts", len(res.Document))
	}
	if out := os.Getenv("CRONOS_PDF_OUT"); out != "" {
		if err := os.WriteFile(out, res.Document, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d KB)", out, len(res.Document)/1024)
	}
}
