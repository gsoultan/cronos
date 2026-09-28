package run_test

import (
	"context"
	"encoding/json"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/app/run"
)

/*
A histogram and a box plot over the million drops the map benchmark reads.
Both read every row, and both are only cheap if the rows stay in the database:
allocs/row is the render's Go allocations over the million rows under it, and
the payload is what a reader's browser is sent.

	go test ./internal/app/run/ -run '^$' -bench Spread -benchtime 3x
*/
func BenchmarkSpreadOfAMillionRows(b *testing.B) {
	s, _ := benchMap(b)
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: spread}
spec:
  dataset: drops
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: chart
          chart: histogram
          title: Parcels a drop
          x: {field: parcels}
        - kind: chart
          chart: boxplot
          title: Parcels a drop, by carrier
          x: {field: carrier}
          y: {field: parcels}`))
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	var payload int
	perRow := allocsPerRow(b, func() {
		v, err := s.Render(ctx, rep, run.Request{}, anyone())
		if err != nil {
			b.Fatal(err)
		}
		if len(v.Blocks[0].Bins) == 0 || len(v.Blocks[1].Boxes) != 5 || v.Blocks[1].Boxes[0].N != benchPlaces/5 {
			b.Fatalf("%d bins and boxes %+v", len(v.Blocks[0].Bins), v.Blocks[1].Boxes)
		}
		raw, _ := json.Marshal(v)
		payload = len(raw)
	})
	b.ReportMetric(perRow, "allocs/row")
	b.ReportMetric(float64(payload), "payload-bytes")
}
