package query

import "math"

// GeoBox is part of the world in degrees: what a reader has in view, padded,
// which a large map's points are narrowed to.
type GeoBox struct {
	South, West, North, East float64
}

// Valid reports whether the box is one a query can be narrowed to: finite,
// on the globe, and the right way round. It arrives from a browser, so the
// alternative is a statement bound with NaN.
func (g GeoBox) Valid() bool {
	for _, v := range []float64{g.South, g.West, g.North, g.East} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return g.South >= -90 && g.North <= 90 && g.South <= g.North &&
		g.West >= -180 && g.East <= 180 && g.West <= g.East
}
