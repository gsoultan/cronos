package run_test

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
An area filter on Postgres, over coordinates stored as NUMERIC.

The query package proves the predicates on SQLite, which has one number type
and takes any arithmetic on it. Postgres has SIN and COS on double precision
only, and every value an area binds arrives as a parameter whose type Postgres
has to infer from the column beside it — so the statement that keeps exactly
the right places on SQLite could be refused here, or compare as text.
*/
func TestAnAreaOnPostgres(t *testing.T) {
	dsn := os.Getenv("CRONOS_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set CRONOS_POSTGRES_DSN — without it an area is only proved on SQLite")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	table := fmt.Sprintf("cronos_area_%d", time.Now().UnixNano())
	if _, err := db.Exec(fmt.Sprintf(`
CREATE TABLE %[1]s (id TEXT, lat NUMERIC(9,6), lon NUMERIC(9,6), carrier TEXT, parcels INTEGER);
INSERT INTO %[1]s
SELECT 'd' || i, 51.5 + (i %% 97) * 0.0003, -0.12 + (i / 97) * 0.0005, 'Aurora', 2
FROM generate_series(0, 4999) AS i;`, table)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table) })
	s := areaService(t, db, table)

	centre := [2]float64{51.5 + 48*0.0003, -0.12 + 25*0.0005}
	near, within := 0, 0
	for i := 0; i < 5000; i++ {
		p := [2]float64{51.5 + float64(i%97)*0.0003, -0.12 + float64(i/97)*0.0005}
		if d := haversine(centre, p); d < 0.5 {
			near++
		}
		// In millionths of a degree, as NUMERIC(9,6) holds them: in floats,
		// -0.12 + 40 × 0.0005 falls short of -0.1, and the column the box's
		// east edge runs down would be counted out of a box it is on.
		lat, lon := 300*(i%97), 500*(i/97)
		if lat >= 10_000 && lat <= 20_000 && lon >= 10_000 && lon <= 20_000 {
			within++
		}
	}
	for name, c := range map[string]struct {
		v    query.FilterValue
		want int
	}{
		"half a kilometre around a place": {query.FilterValue{Op: query.Near,
			Values: []any{centre[0], centre[1], 0.5}}, near},
		"a box of streets": {query.FilterValue{Op: query.Within,
			Values: []any{51.51, -0.11, 51.52, -0.1}}, within},
	} {
		if got := dropsIn(t, s, c.v); got != c.want || got == 0 {
			t.Errorf("%s: %d drops, want %d", name, got, c.want)
		}
	}
}

// dropsIn is how many drops the report counts under the area.
func dropsIn(t *testing.T, s *run.Service, v query.FilterValue) int {
	t.Helper()
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: area-postgres}
spec:
  dataset: drops
  filters:
    - {name: where, type: area, bind: {drops: "lat,lon"}}
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - {kind: stat, label: Drops, value: {field: id, aggregate: count}}`))
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.Render(context.Background(), rep,
		run.Request{Filters: map[string]query.FilterValue{"where": v}}, anyone())
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.ReplaceAll(view.Blocks[0].Value, ",", ""))
	if err != nil {
		t.Fatalf("a count of %q", view.Blocks[0].Value)
	}
	return n
}

func areaService(t *testing.T, db *sql.DB, table string) *run.Service {
	t.Helper()
	ds, err := yamlcodec.Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: drops}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, lat, lon, parcels FROM ` + table + `
  fields:
    - {name: id,      type: string,  role: dimension}
    - {name: lat,     type: decimal, role: dimension}
    - {name: lon,     type: decimal, role: dimension}
    - {name: parcels, type: decimal, role: measure, aggregate: sum}`))
	if err != nil {
		t.Fatal(err)
	}
	return run.New(datasets{"drops": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.Postgres{}),
	}})
}

// haversine is the distance between two places in kilometres, on the sphere
// the filter measures on.
func haversine(a, b [2]float64) float64 {
	r := math.Pi / 180
	dLat, dLon := (b[0]-a[0])*r, (b[1]-a[1])*r
	h := math.Pow(math.Sin(dLat/2), 2) + math.Cos(a[0]*r)*math.Cos(b[0]*r)*math.Pow(math.Sin(dLon/2), 2)
	return 2 * 6371 * math.Asin(math.Sqrt(h))
}
