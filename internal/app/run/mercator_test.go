package run

import (
	"math"
	"testing"
)

// close reports whether a and b agree to within a world unit's worth of the
// resolution the payload actually carries.
func close(a, b float64) bool { return math.Abs(a-b) < 1e-5 }

func TestTheProjectionIsTheOneTileServersPublishIn(t *testing.T) {
	// The whole basemap design rests on this: our world units and an XYZ tile
	// grid are the same space, so a viewer places tiles with arithmetic rather
	// than with a projection library. These are the anchors that hold if — and
	// only if — this really is Web Mercator normalised to the unit square.
	for _, c := range []struct {
		name     string
		lon, lat float64
		x, y     float64
	}{
		{"null island is the centre", 0, 0, 0.5, 0.5},
		{"the antimeridian is the left edge", -180, 0, 0, 0.5},
		{"the northern cut is the top edge", 0, MercatorLimit, 0.5, 0},
		{"the southern cut is the bottom edge", 0, -MercatorLimit, 0.5, 1},
		// Greenwich, which is the one coordinate anybody can check by hand.
		{"london", -0.1278, 51.5074, 0.4996450, 0.3325255},
	} {
		t.Run(c.name, func(t *testing.T) {
			x, y := project(c.lon, c.lat)
			if !close(x, c.x) || !close(y, c.y) {
				t.Errorf("project(%v, %v) = %.7f, %.7f — want %.7f, %.7f",
					c.lon, c.lat, x, y, c.x, c.y)
			}
		})
	}
}

func TestThePolesAreClampedRatherThanSentToInfinity(t *testing.T) {
	// Mercator sends 90° to infinity. A polygon reaching into the Arctic is
	// ordinary data, and an infinite coordinate in an SVG path is a shape that
	// fills the whole map — which reads as the largest region rather than as
	// a broken one.
	for _, lat := range []float64{90, -90, 89.9} {
		if _, y := project(0, lat); math.IsInf(y, 0) || math.IsNaN(y) || y < 0 || y > 1 {
			t.Errorf("project(0, %v) gave y = %v, which is not on the map", lat, y)
		}
	}
}

func TestACoordinateThatIsNotOneIsRefused(t *testing.T) {
	for _, c := range []struct {
		name     string
		lon, lat float64
	}{
		{"longitude past the antimeridian", 181, 0},
		{"latitude past the pole", 0, 91},
		{"not a number", math.NaN(), 0},
		{"infinite", math.Inf(1), 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if finite(c.lon, c.lat) {
				t.Errorf("finite(%v, %v) accepted a coordinate that is not one", c.lon, c.lat)
			}
		})
	}
}

func TestSimplifyingKeepsTheShapeAndDropsTheVertices(t *testing.T) {
	// A straight run of points with one real corner. Every point on the runs
	// is redundant; the corner is the shape.
	ring := [][2]float64{{0, 0}}
	for i := 1; i <= 50; i++ {
		ring = append(ring, [2]float64{float64(i) / 100, 0})
	}
	ring = append(ring, [2]float64{0.5, 0.5}, [2]float64{0, 0})

	out := simplify(ring, 0.001)
	if len(out) >= len(ring) {
		t.Errorf("simplify kept %d of %d points — the straight run should collapse",
			len(out), len(ring))
	}
	if out[0] != ring[0] || out[len(out)-1] != ring[len(ring)-1] {
		t.Error("simplify moved an end of the ring, which unseals the polygon")
	}
	// The corner is the one interior point that has to survive.
	var corner bool
	for _, p := range out {
		if p == [2]float64{0.5, 0.5} {
			corner = true
		}
	}
	if !corner {
		t.Error("simplify dropped the corner, which is the only shape the ring had")
	}
}

func TestSimplifyingNeverThinsARingBelowAShape(t *testing.T) {
	// A tolerance larger than the shape would otherwise leave two points,
	// which draws as a stray line across the map rather than as a small
	// region — a failure that looks like a rendering bug and is not.
	ring := [][2]float64{{0, 0}, {0.001, 0}, {0.001, 0.001}, {0, 0.001}, {0, 0}}
	if out := simplify(ring, 0.5); len(out) < 4 {
		t.Errorf("simplify left %d points, which is not a closed shape", len(out))
	}
}

func TestASinglePointStillGetsAMapToSitOn(t *testing.T) {
	// A viewBox of zero width renders nothing, and nothing looks exactly like
	// a report that matched no rows.
	b := newBounds()
	x, y := project(-0.1278, 51.5074)
	b.add(x, y)

	got := b.padded()
	if got.MaxX-got.MinX <= 0 || got.MaxY-got.MinY <= 0 {
		t.Errorf("one marker gave a collapsed box: %+v", got)
	}
	if got.MinX < 0 || got.MinY < 0 || got.MaxX > 1 || got.MaxY > 1 {
		t.Errorf("padding pushed the box off the world: %+v", got)
	}
}

func TestAMapWithNothingOnItFallsBackToTheWholeWorld(t *testing.T) {
	if got := newBounds().padded(); got != wholeWorld {
		t.Errorf("an empty map gave %+v, want the whole world", got)
	}
}
