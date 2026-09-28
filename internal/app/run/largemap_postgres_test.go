package run_test

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
A large map on Postgres, over the column types a warehouse actually has:
coordinates as NUMERIC and a measure as INTEGER.

SQLite has one number type and accepts any arithmetic on it, so everything the
large map writes in SQL — the projection, the floor of a cell, a hexagon's cube
rounding — could be wrong for Postgres and every other test here would pass.
NUMERIC in particular: SIN and LN are defined on double precision, and a
statement that leans on an implicit cast is one a stricter database may not make.
*/
func TestALargeMapOnPostgres(t *testing.T) {
	dsn := os.Getenv("CRONOS_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CRONOS_POSTGRES_DSN — without it the large map's SQL is only proved on SQLite")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	table := fmt.Sprintf("cronos_largemap_%d", time.Now().UnixNano())
	places := query.ChartLimit + 700
	if _, err := db.Exec(fmt.Sprintf(`
CREATE TABLE %[1]s (id TEXT, lat NUMERIC(9,6), lon NUMERIC(9,6), carrier TEXT, parcels INTEGER);
INSERT INTO %[1]s
SELECT 'd' || i, 51.5 + (i %% 97) * 0.0003, -0.12 + (i / 97) * 0.0005,
       CASE WHEN i %% 3 = 0 THEN 'Baltic' ELSE 'Aurora' END, 2
FROM generate_series(1, %[2]d) AS i;`, table, places)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table) })

	s, rep := postgresMap(t, db, table)
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	dots, hexes := v.Blocks[0].Map, v.Blocks[1].Map
	if dots.Places != places || dots.Cells == nil || len(dots.Keys) != 2 {
		t.Fatalf("the dots: %d places of %d, cells %v, keys %+v",
			dots.Places, places, dots.Cells != nil, dots.Keys)
	}
	total := 0.0
	for _, h := range hexes.Hexes {
		total += h.Value
	}
	if total != float64(2*places) {
		t.Errorf("the hexagons hold %v parcels, want %d", total, 2*places)
	}
	postgresView(t, s, rep, dots)
}

// postgresView zooms into the first column of drops: each cell one drop, where
// Go would have put it, with its own value and its carrier's colour.
func postgresView(t *testing.T, s *run.Service, rep definition.Report, m *run.GeoMap) {
	t.Helper()
	x0, y0 := gridAt(0)
	x1, y1 := gridAt(96)
	got, err := s.MapView(context.Background(), rep, run.ViewRequest{
		Block: 0, Width: 1600, Height: 1600, Categories: m.Detail.Categories,
		View: run.Bounds{MinX: x0 - 1e-6, MinY: min(y0, y1), MaxX: x1 + 1e-6, MaxY: max(y0, y1)},
	}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for i, label := range got.Cells.L {
		var d int
		if _, err := fmt.Sscanf(label, "d%d", &d); err != nil {
			continue
		}
		wx, wy := gridAt(d)
		if math.Abs(got.Cells.X[i]-wx) > 1e-9 || math.Abs(got.Cells.Y[i]-wy) > 1e-9 {
			t.Errorf("drop %d is at (%v, %v), Go puts it at (%v, %v)", d, got.Cells.X[i], got.Cells.Y[i], wx, wy)
		}
		want := 0
		if d%3 == 0 {
			want = 1
		}
		if got.Cells.V[i] != 2 || got.Cells.S[i] != want {
			t.Errorf("drop %d: value %v, swatch %d", d, got.Cells.V[i], got.Cells.S[i])
		}
		checked++
	}
	if checked < 90 {
		t.Errorf("only %d of the column's drops came back as themselves", checked)
	}
}

func postgresMap(t testing.TB, db *sql.DB, table string) (*run.Service, definition.Report) {
	t.Helper()
	ds, err := yamlcodec.Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: drops}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, lat, lon, carrier, parcels FROM ` + table + `
  fields:
    - {name: id,      type: string,  role: dimension}
    - {name: lat,     type: decimal, role: dimension}
    - {name: lon,     type: decimal, role: dimension}
    - {name: carrier, type: string,  role: dimension}
    - {name: parcels, type: decimal, role: measure, aggregate: sum}`))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: drops-postgres}
spec:
  dataset: drops
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: chart
          chart: map
          title: Drops
          x: {field: id}
          y: {field: parcels, aggregate: sum}
          series: {field: carrier}
          map: {layers: [scatter], lat: lat, lon: lon}
        - kind: chart
          chart: map
          title: Drops per hexagon
          x: {field: id}
          y: {field: parcels, aggregate: sum}
          map: {layers: [hexbin], lat: lat, lon: lon, hexKm: 1}`))
	if err != nil {
		t.Fatal(err)
	}
	return run.New(datasets{"drops": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.Postgres{}),
	}}), rep
}

/*
The million-place map on Postgres, which is where a deployment keeps one:
hashing where SQLite sorts, so the same statements run in a fraction of the
time. Skipped without CRONOS_POSTGRES_DSN, like the test above.

	CRONOS_POSTGRES_DSN=… go test ./internal/app/run/ -run '^$' -bench LargeMapOnPostgres
*/
func BenchmarkLargeMapOnPostgres(b *testing.B) {
	dsn := os.Getenv("CRONOS_POSTGRES_DSN")
	if dsn == "" {
		b.Skip("set CRONOS_POSTGRES_DSN")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { db.Close() })
	table := fmt.Sprintf("cronos_largemap_bench_%d", time.Now().UnixNano())
	if _, err := db.Exec(fmt.Sprintf(`
CREATE TABLE %[1]s (id TEXT, lat DOUBLE PRECISION, lon DOUBLE PRECISION, carrier TEXT, parcels INTEGER);
INSERT INTO %[1]s
SELECT 'd' || i, 50.8 + ((i * 7919::bigint) %% 100003) / 100003.0 * 2.7,
       3.4 + ((i * 104729::bigint) %% 99991) / 99991.0 * 3.8,
       (ARRAY['Aurora','Baltic','Cobalt','Delta','Ember'])[1 + i %% 5], 1 + i %% 7
FROM generate_series(1, %[2]d) AS i;
ANALYZE %[1]s;`, table, benchPlaces)); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table) })
	s, rep := postgresMap(b, db, table)
	rep.Outputs[0].Layout = rep.Outputs[0].Layout[:1]

	perRow := allocsPerRow(b, func() {
		v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
		if err != nil {
			b.Fatal(err)
		}
		if m := v.Blocks[0].Map; m.Places != benchPlaces {
			b.Fatalf("%d places", m.Places)
		}
	})
	b.ReportMetric(perRow, "allocs/row")
}
