package run

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"

	_ "modernc.org/sqlite"
)

/*
The database and this package must agree on where a place is.

A large map is projected and binned in SQL; a small one in Go. A reader zooming
from one to the other — or comparing a filtered map with the unfiltered one —
sees the same place in the same spot and the same hexagon only if the two
compute the same numbers. These run the SQL the builder writes against SQLite
and compare it with project and cellOf, place by place.
*/

var pointDB = 0

// dbExecutor runs a plan on a database, which is all these need of the SQL
// driver package — and that package imports this one.
type dbExecutor struct{ db *sql.DB }

func (e dbExecutor) Execute(ctx context.Context, p query.Plan) (Rows, error) {
	return e.db.QueryContext(ctx, p.SQL(), p.Args()...)
}

// pointsTable is a table of places at the coordinates given, and a map block
// over it.
func pointsTable(t *testing.T, coords [][2]float64) (*bigMap, *sql.DB) {
	t.Helper()
	pointDB++
	db, err := sql.Open("sqlite", fmt.Sprintf("file:points%d?mode=memory&cache=shared", pointDB))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var b strings.Builder
	b.WriteString("CREATE TABLE places (id TEXT, lat REAL, lon REAL, n REAL);\n")
	for i, c := range coords {
		fmt.Fprintf(&b, "INSERT INTO places VALUES ('p%d', %.12f, %.12f, 1);\n", i, c[0], c[1])
	}
	if _, err := db.Exec(b.String()); err != nil {
		t.Fatal(err)
	}
	ds := definition.Dataset{
		Name: "places", Query: "SELECT id, lat, lon, n FROM places",
		Fields: []definition.Field{
			{Name: "id", Type: "string", Role: definition.Dimension},
			{Name: "lat", Type: "decimal", Role: definition.Dimension},
			{Name: "lon", Type: "decimal", Role: definition.Dimension},
			{Name: "n", Type: "decimal", Role: definition.Measure, Aggregate: "sum"},
		},
	}
	blk := definition.Block{
		Kind: definition.ChartBlock, Chart: definition.MapChart,
		X: definition.DimensionRef{Field: "id"}, Y: definition.MeasureRef{Field: "n", Aggregate: "sum"},
		Map: &definition.MapSpec{Layers: []definition.MapLayer{definition.HexbinLayer}, Lat: "lat", Lon: "lon"},
	}
	engine := Engine{Executor: dbExecutor{db}, Builder: query.NewBuilder(query.SQLite{})}
	return &bigMap{blk: blk, ds: ds, engine: engine, pr: principal.Principal{Member: true,
		ProjectRole: principal.ProjectEditor}}, db
}

// Through the cells, at a grid fine enough that each holds one place: a cell's
// middle is then that place, as the database projected it.
func TestTheDatabaseProjectsAPlaceWhereGoDoes(t *testing.T) {
	coords := [][2]float64{
		{51.5074, -0.1278}, {-33.8688, 151.2093}, {0, 0}, {89.9, 179.99}, {-89.9, -179.99},
		{MercatorLimit, 10}, {-MercatorLimit, -10}, {85.2, 1}, {64.1466, -21.9426},
	}
	q, _ := pointsTable(t, coords)
	q.blk.Map.Layers = []definition.MapLayer{definition.ScatterLayer}
	c, err := q.cells(context.Background(), query.MapGrid{Cells: 1 << 30}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.X) != len(coords) {
		t.Fatalf("%d cells for %d places, each far from the rest", len(c.X), len(coords))
	}
	for i, label := range c.L {
		var at int
		fmt.Sscanf(label, "p%d", &at)
		wx, wy := project(coords[at][1], coords[at][0])
		if math.Abs(c.X[i]-wx) > 1e-12 || math.Abs(c.Y[i]-wy) > 1e-9 {
			t.Errorf("(%v, %v): the database put it at (%.15f, %.15f), Go at (%.15f, %.15f)",
				coords[at][0], coords[at][1], c.X[i], c.Y[i], wx, wy)
		}
	}
}

// Every place lands in the hexagon Go would have put it in, and each hexagon
// counts the places Go would have counted.
func TestTheDatabaseBinsHexagonsTheWayGoDoes(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	coords := make([][2]float64, 3000)
	for i := range coords {
		coords[i] = [2]float64{52 + rng.Float64()*1.5, 4 + rng.Float64()*2.5}
	}
	q, _ := pointsTable(t, coords)
	radius := 0.0009

	want := map[hexCell]int{}
	for _, c := range coords {
		x, y := project(c[1], c[0])
		want[cellOf(x, y, radius)]++
	}
	m := emptyMap(q.blk)
	if err := q.hexes(context.Background(), m, radius, newBounds()); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, h := range m.Hexes {
		got[h.Path] = h.Label
	}
	if len(got) != len(want) {
		t.Fatalf("the database made %d hexagons, Go %d", len(got), len(want))
	}
	for c, n := range want {
		if l := got[hexPath(c, radius, newBounds())]; l != places(n) {
			t.Errorf("hexagon %v holds %q in the database, %q in Go", c, l, places(n))
		}
	}
}
