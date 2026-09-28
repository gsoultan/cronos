package query

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"

	_ "modernc.org/sqlite"
)

/*
An area filter, run against the database it compiles for: every place the
database keeps is inside the area by Go's own arithmetic, and every place
inside it is kept. A predicate that compiled and selected the wrong rows would
pass any test that only read the SQL.
*/

var areaDB = 0

// places is a table of points on a grid around Amsterdam, and a dataset over
// it with an area filter bound to its coordinates.
func places(t *testing.T, extra ...[2]float64) (*sql.DB, definition.Dataset, [][2]float64) {
	t.Helper()
	areaDB++
	db, err := sql.Open("sqlite", fmt.Sprintf("file:area%d?mode=memory&cache=shared", areaDB))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var pts [][2]float64
	for i := -25; i <= 25; i++ {
		for j := -25; j <= 25; j++ {
			pts = append(pts, [2]float64{52.37 + float64(i)*0.02, 4.90 + float64(j)*0.02})
		}
	}
	pts = append(pts, extra...)
	var b strings.Builder
	b.WriteString("CREATE TABLE places (id INTEGER, lat REAL, lon REAL);\n")
	for i, p := range pts {
		fmt.Fprintf(&b, "INSERT INTO places VALUES (%d, %.9f, %.9f);\n", i, p[0], p[1])
	}
	if _, err := db.Exec(b.String()); err != nil {
		t.Fatal(err)
	}
	ds := definition.Dataset{Name: "places", Query: "SELECT id, lat, lon FROM places",
		Fields: []definition.Field{
			{Name: "id", Type: "number", Role: definition.Dimension},
			{Name: "lat", Type: "decimal", Role: definition.Dimension},
			{Name: "lon", Type: "decimal", Role: definition.Dimension},
		}}
	return db, ds, pts
}

func areaFilter() definition.Filter {
	return definition.Filter{Name: "where", Type: definition.Area,
		Bind: map[string]string{"places": "lat,lon"}}
}

// kept runs the dataset under the filter and returns the ids it keeps.
func kept(t *testing.T, db *sql.DB, ds definition.Dataset, v FilterValue) map[int]bool {
	t.Helper()
	plan, _, err := NewBuilder(SQLite{}).BuildWith(ds, nil,
		Filters{Defs: []definition.Filter{areaFilter()}, Values: map[string]FilterValue{"where": v}},
		embedded("c-9"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(plan.SQL(), plan.Args()...)
	if err != nil {
		t.Fatalf("%v\n%s", err, plan.SQL())
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var id int
		var lat, lon float64
		if err := rows.Scan(&id, &lat, &lon); err != nil {
			t.Fatal(err)
		}
		out[id] = true
	}
	return out
}

func haversineKm(a, b [2]float64) float64 {
	r := math.Pi / 180
	dLat, dLon := (b[0]-a[0])*r, (b[1]-a[1])*r
	h := math.Pow(math.Sin(dLat/2), 2) + math.Cos(a[0]*r)*math.Cos(b[0]*r)*math.Pow(math.Sin(dLon/2), 2)
	return 2 * 6371 * math.Asin(math.Sqrt(h))
}

func TestNearKeepsExactlyThePlacesWithinTheDistance(t *testing.T) {
	db, ds, pts := places(t)
	centre := [2]float64{52.37, 4.90}
	got := kept(t, db, ds, FilterValue{Op: Near, Values: []any{52.37, 4.90, 10.0}})
	inside := 0
	for i, p := range pts {
		d := haversineKm(centre, p)
		// A metre of slack either side, for the spherical Earth both assume.
		switch {
		case d < 9.999 && !got[i]:
			t.Errorf("(%.2f, %.2f) is %.3f km away and was left out", p[0], p[1], d)
		case d > 10.001 && got[i]:
			t.Errorf("(%.2f, %.2f) is %.3f km away and was kept", p[0], p[1], d)
		}
		if d < 10 {
			inside++
		}
	}
	if inside < 100 {
		t.Fatalf("only %d places within 10 km — the test proves little", inside)
	}
}

/*
A place just inside the distance is kept whichever way it lies, and one just
outside is not — at the edge of the circle, where the box that lets an index
be used is tightest. The box once used 111.32 km to a degree of latitude, on a
sphere whose degree is 111.19: a place 9.99 km due north of the centre was
outside it, and so never reached the comparison that would have kept it.
*/
func TestNearKeepsAPlaceAtTheEdgeInEveryDirection(t *testing.T) {
	for _, centre := range [][2]float64{{0.5, 10}, {52.37, 4.90}, {69.65, 18.96}, {89.95, 0}} {
		var edge [][2]float64
		for _, bearing := range []float64{0, 45, 90, 135, 180, 225, 270, 315} {
			edge = append(edge, towards(centre, 9.99, bearing), towards(centre, 10.01, bearing))
		}
		db, ds, pts := places(t, edge...)
		got := kept(t, db, ds, FilterValue{Op: Near, Values: []any{centre[0], centre[1], 10.0}})
		for i := len(pts) - len(edge); i < len(pts); i++ {
			if inside := (i-len(pts)+len(edge))%2 == 0; got[i] != inside {
				t.Errorf("around (%.2f, %.2f), (%.5f, %.5f) is %.3f km away: kept %v",
					centre[0], centre[1], pts[i][0], pts[i][1], haversineKm(centre, pts[i]), got[i])
			}
		}
	}
}

// towards is where km along a bearing from a place lands, on the sphere.
func towards(from [2]float64, km, bearing float64) [2]float64 {
	r := math.Pi / 180
	d, theta, phi := km/6371, bearing*r, from[0]*r
	lat := math.Asin(math.Sin(phi)*math.Cos(d) + math.Cos(phi)*math.Sin(d)*math.Cos(theta))
	lon := from[1]*r + math.Atan2(math.Sin(theta)*math.Sin(d)*math.Cos(phi), math.Cos(d)-math.Sin(phi)*math.Sin(lat))
	return [2]float64{lat / r, math.Mod(lon/r+540, 360) - 180}
}

func TestWithinKeepsTheBoxAndCrossesTheAntimeridian(t *testing.T) {
	db, ds, pts := places(t, [2]float64{0, 179.5}, [2]float64{0, -179.5}, [2]float64{0, 0})
	box := kept(t, db, ds, FilterValue{Op: Within, Values: []any{52.3, 4.8, 52.4, 5.0}})
	for i, p := range pts {
		want := p[0] >= 52.3 && p[0] <= 52.4 && p[1] >= 4.8 && p[1] <= 5.0
		if box[i] != want {
			t.Errorf("(%.2f, %.2f): kept %v, want %v", p[0], p[1], box[i], want)
		}
	}
	// West of east: the box runs from 179° east across the date line to 179°
	// west, and takes both sides of it and nothing between.
	wrap := kept(t, db, ds, FilterValue{Op: Within, Values: []any{-1.0, 179.0, 1.0, -179.0}})
	n := len(pts)
	if !wrap[n-3] || !wrap[n-2] || wrap[n-1] {
		t.Errorf("across the antimeridian kept %v", wrap)
	}
}

// Every number is a caller's, so every wrong one is refused with a sentence,
// and an area's operators belong to an area alone.
func TestAnAreaIsRefusedWhatItCannotMean(t *testing.T) {
	_, ds, _ := places(t)
	build := func(def definition.Filter, v FilterValue) error {
		_, _, err := NewBuilder(SQLite{}).BuildWith(ds, nil,
			Filters{Defs: []definition.Filter{def}, Values: map[string]FilterValue{def.Name: v}},
			embedded("c-9"))
		return err
	}
	date := definition.Filter{Name: "when", Type: definition.Date, Bind: map[string]string{"places": "lat"}}
	for name, c := range map[string]struct {
		def definition.Filter
		v   FilterValue
	}{
		"a box off the globe":       {areaFilter(), FilterValue{Op: Within, Values: []any{0.0, 0.0, 95.0, 1.0}}},
		"a box upside down":         {areaFilter(), FilterValue{Op: Within, Values: []any{10.0, 0.0, 5.0, 1.0}}},
		"a distance of nothing":     {areaFilter(), FilterValue{Op: Near, Values: []any{52.0, 4.0, 0.0}}},
		"not a number":              {areaFilter(), FilterValue{Op: Near, Values: []any{"x", 4.0, 5.0}}},
		"not finite":                {areaFilter(), FilterValue{Op: Near, Values: []any{math.NaN(), 4.0, 5.0}}},
		"too few numbers":           {areaFilter(), FilterValue{Op: Near, Values: []any{52.0, 4.0}}},
		"an area compared as equal": {areaFilter(), FilterValue{Op: Eq, Values: []any{52.0}}},
		"a date sent near":          {date, FilterValue{Op: Near, Values: []any{52.0, 4.0, 5.0}}},
	} {
		if err := build(c.def, c.v); !errors.Is(err, ErrBadArgument) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
