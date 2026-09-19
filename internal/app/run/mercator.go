package run

import "math"

// MercatorLimit is the latitude beyond which Web Mercator stops being finite.
//
// The projection sends the poles to infinity, so every slippy map in existence
// cuts the world at this latitude to make it a square. Clamping to it rather
// than dropping the vertex keeps a polygon that reaches into the Arctic a
// closed shape instead of an open path that fills across the map.
const MercatorLimit = 85.05112878

// project converts a coordinate to Web Mercator world units, where the whole
// world is the unit square: x 0 at 180°W, y 0 at the northern cut.
//
// Normalised rather than metres, and Web Mercator rather than something with
// less distortion, because both decisions are really one decision: this is the
// projection OSM, Google, Mapbox and every XYZ tileserver already publish in.
// Sharing it means a tile at (z, x, y) covers exactly the world-unit square
// [x/2^z, (x+1)/2^z) — so a viewer can lay tiles under our geometry with two
// multiplications and no projection code, and no map library, of its own.
func project(lon, lat float64) (x, y float64) {
	lat = math.Max(-MercatorLimit, math.Min(MercatorLimit, lat))
	rad := lat * math.Pi / 180
	y = 0.5 - math.Log(math.Tan(rad)+1/math.Cos(rad))/(2*math.Pi)

	// Clamped, because the arithmetic misses. MercatorLimit is the latitude
	// whose y is zero, and evaluating it in float64 lands six picometres below
	// — so a polygon touching the Arctic yields y = -6e-12, and a viewer
	// turning that into a tile index gets tile -1 and a gap along the top of
	// the map. The unit square is the contract; this is what makes it true.
	return clamp01((lon + 180) / 360), clamp01(y)
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// finite reports whether a coordinate is one a map can draw.
//
// A NULL latitude arrives as a zero, and (0, 0) is the Gulf of Guinea — a real
// place that a thousand rows with a missing address will cluster on. Out-of-
// range values are rejected outright; null island is left to the caller, which
// knows whether the row had a coordinate at all.
func finite(lon, lat float64) bool {
	return !math.IsNaN(lon) && !math.IsNaN(lat) &&
		!math.IsInf(lon, 0) && !math.IsInf(lat, 0) &&
		lon >= -180 && lon <= 180 && lat >= -90 && lat <= 90
}

// simplify drops vertices a Douglas-Peucker pass finds redundant at tolerance.
//
// Tolerance is in world units, so it is a constant fraction of the map rather
// than of the shape: the same number thins a country and a city block by the
// same amount of drawn detail, which is what a reader at a fixed pixel size
// actually perceives.
func simplify(ring [][2]float64, tolerance float64) [][2]float64 {
	if tolerance <= 0 || len(ring) < 3 {
		return ring
	}
	keep := make([]bool, len(ring))
	keep[0], keep[len(ring)-1] = true, true
	mark(ring, 0, len(ring)-1, tolerance*tolerance, keep)

	out := make([][2]float64, 0, len(ring))
	for i, k := range keep {
		if k {
			out = append(out, ring[i])
		}
	}
	// A ring thinned past a triangle is not a shape. Returning the original is
	// better than returning a sliver that renders as a stray line across the
	// map, and the caller already capped how much detail it asked to lose.
	if len(out) < 4 {
		return ring
	}
	return out
}

// mark recursively keeps the furthest vertex from the chord while it is
// further than the tolerance allows.
func mark(ring [][2]float64, first, last int, sqTolerance float64, keep []bool) {
	worst, at := sqTolerance, -1
	for i := first + 1; i < last; i++ {
		if d := sqSegDist(ring[i], ring[first], ring[last]); d > worst {
			worst, at = d, i
		}
	}
	if at < 0 {
		return
	}
	keep[at] = true
	mark(ring, first, at, sqTolerance, keep)
	mark(ring, at, last, sqTolerance, keep)
}

// sqSegDist is the squared distance from p to the segment ab.
//
// Squared throughout: the comparison is the only thing the caller needs and a
// square root per vertex on a continent's coastline is a cost with no reader.
func sqSegDist(p, a, b [2]float64) float64 {
	x, y := a[0], a[1]
	dx, dy := b[0]-x, b[1]-y
	if dx != 0 || dy != 0 {
		t := ((p[0]-x)*dx + (p[1]-y)*dy) / (dx*dx + dy*dy)
		switch {
		case t > 1:
			x, y = b[0], b[1]
		case t > 0:
			x, y = x+dx*t, y+dy*t
		}
	}
	dx, dy = p[0]-x, p[1]-y
	return dx*dx + dy*dy
}
