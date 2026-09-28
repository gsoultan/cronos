package run_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"runtime"
	"sync"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
A map over a million places, end to end: the query that finds it will not fit,
the survey, the categories, the cells — everything a reader's first look at it
costs — and then a reader zooming into one neighbourhood.

What the perf profile asks of the data plane is that a row costs nothing it does
not have to: allocs/row is the Go allocations of the whole render divided by the
million rows under it, which is only small if the rows never reach Go. The
payload is reported too, because a map that renders in a second and sends forty
megabytes has moved the problem to the reader's browser.

	go test ./internal/app/run/ -run '^$' -bench LargeMap -benchtime 5x
*/

const benchPlaces = 1_000_000

var (
	benchOnce sync.Once
	benchSvc  *run.Service
	benchRep  definition.Report
	benchDB   *sql.DB
	benchDS   definition.Dataset
)

// benchMap is a million drops across the Netherlands, five carriers, seeded
// by arithmetic so every run is the same map.
func benchMap(b *testing.B) (*run.Service, definition.Report) {
	b.Helper()
	benchOnce.Do(func() {
		db, err := sql.Open("sqlite", "file:bench-map?mode=memory&cache=shared")
		if err != nil {
			b.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		if _, err := db.Exec(`
CREATE TABLE drops (id TEXT, lat REAL, lon REAL, carrier TEXT, parcels REAL);
WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 1000000)
INSERT INTO drops
SELECT 'd' || i,
       50.8 + ((i * 7919) % 100003) / 100003.0 * 2.7,
       3.4 + ((i * 104729) % 99991) / 99991.0 * 3.8,
       CASE i % 5 WHEN 0 THEN 'Aurora' WHEN 1 THEN 'Baltic' WHEN 2 THEN 'Cobalt'
                  WHEN 3 THEN 'Delta' ELSE 'Ember' END,
       1 + i % 7
FROM n;`); err != nil {
			b.Fatal(err)
		}
		ds, err := yamlcodec.Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: drops}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, lat, lon, carrier, parcels FROM drops
  fields:
    - {name: id,      type: string,  role: dimension}
    - {name: lat,     type: decimal, role: dimension}
    - {name: lon,     type: decimal, role: dimension}
    - {name: carrier, type: string,  role: dimension}
    - {name: parcels, type: decimal, role: measure, aggregate: sum}`))
		if err != nil {
			b.Fatal(err)
		}
		benchRep, err = yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: million}
spec:
  dataset: drops
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: chart
          chart: map
          title: A million drops
          x: {field: id}
          y: {field: parcels, aggregate: sum}
          series: {field: carrier}
          map: {layers: [scatter], lat: lat, lon: lon}`))
		if err != nil {
			b.Fatal(err)
		}
		benchDB, benchDS = db, ds
		benchSvc = run.New(datasets{"drops": ds}, run.One{Only: run.Engine{
			Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
		}})
	})
	return benchSvc, benchRep
}

func BenchmarkLargeMapOpens(b *testing.B) {
	s, rep := benchMap(b)
	ctx := context.Background()
	var payload int
	perRow := allocsPerRow(b, func() {
		v, err := s.Render(ctx, rep, run.Request{}, anyone())
		if err != nil {
			b.Fatal(err)
		}
		m := v.Blocks[0].Map
		if m.Places != benchPlaces || m.Cells == nil {
			b.Fatalf("a million drops came back as %d places, cells %v", m.Places, m.Cells != nil)
		}
		raw, _ := json.Marshal(m)
		payload = len(raw)
	})
	b.ReportMetric(perRow, "allocs/row")
	b.ReportMetric(float64(payload), "payload-bytes")
}

func BenchmarkLargeMapZoomsIn(b *testing.B) {
	s, rep := benchMap(b)
	ctx := context.Background()
	// A few streets of Amsterdam: the view a reader reaches in five clicks.
	x0, y0 := world(52.38, 4.88)
	x1, y1 := world(52.36, 4.92)
	req := run.ViewRequest{Block: 0, Width: 1200, Height: 800,
		View:       run.Bounds{MinX: x0, MinY: y0, MaxX: x1, MaxY: y1},
		Categories: []string{"Aurora", "Baltic", "Cobalt", "Delta", "Ember"}}
	var payload int
	perRow := allocsPerRow(b, func() {
		m, err := s.MapView(ctx, rep, req, anyone())
		if err != nil {
			b.Fatal(err)
		}
		raw, _ := json.Marshal(m)
		payload = len(raw)
	})
	b.ReportMetric(perRow, "allocs/row")
	b.ReportMetric(float64(payload), "payload-bytes")
}

// allocsPerRow runs fn b.N times and divides what it allocated by the rows
// under the map.
func allocsPerRow(b *testing.B, fn func()) float64 {
	b.Helper()
	b.ReportAllocs()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	b.ResetTimer()
	for range b.N {
		fn()
	}
	b.StopTimer()
	runtime.ReadMemStats(&after)
	return float64(after.Mallocs-before.Mallocs) / float64(b.N) / benchPlaces
}
