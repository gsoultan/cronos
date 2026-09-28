package run

import (
	"math"

	"github.com/gsoultan/cronos/internal/core/document"
)

// ringSides is how many sides a printed radius circle has: round at any size
// a report prints a map at.
const ringSides = 48

/*
rings prints a radius layer: a circle of km around each place, measured on the
ground, as a wash under its outline — so where two catchments overlap reads as
overlap. A large map's cells are printed as rings only where a cell is one
place; a circle around a crowd would be an area nobody serves.
*/
func rings(c *document.Chart, m *GeoMap, at func(x, y float64) [2]float64, keyed bool) {
	ring := func(x, y float64, slot int) {
		pts := geodesic(latitude(y), x*360-180, m.RadiusKm, ringSides)
		for i, p := range pts {
			pts[i] = at(p[0], p[1])
		}
		colour := "series-2"
		if keyed {
			colour = tone(slot)
		}
		c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark, Points: pts, Tone: colour + "-wash"},
			document.Mark{Kind: document.LineMark, Points: append(pts, pts[0]), Tone: colour})
	}
	for _, p := range m.Markers {
		ring(p.X, p.Y, p.Slot)
	}
	if m.Cells == nil {
		return
	}
	for i, n := range m.Cells.N {
		if n == 1 {
			slot := 0
			if i < len(m.Cells.S) {
				slot = m.Cells.S[i]
			}
			ring(m.Cells.X[i], m.Cells.Y[i], slot)
		}
	}
}

// geodesic is the circle of km around a place, in world units: n points where
// a bearing of km along the sphere lands, kept on the centre's side of the
// antimeridian so a circle over it is a circle rather than a band.
func geodesic(lat, lon, km float64, n int) [][2]float64 {
	const r = math.Pi / 180
	d := km / 6371
	phi := lat * r
	out := make([][2]float64, 0, n)
	for i := 0; i < n; i++ {
		theta := float64(i) / float64(n) * 2 * math.Pi
		to := math.Asin(math.Sin(phi)*math.Cos(d) + math.Cos(phi)*math.Sin(d)*math.Cos(theta))
		along := lon*r + math.Atan2(math.Sin(theta)*math.Sin(d)*math.Cos(phi),
			math.Cos(d)-math.Sin(phi)*math.Sin(to))
		// x unclamped: project keeps a place inside the world, which is the
		// wrong thing for a circle reaching over its edge.
		_, y := project(along/r, to/r)
		out = append(out, [2]float64{(along/r + 180) / 360, y})
	}
	return out
}

// arrowhead is a flow's head: a small triangle at q, pointing from ctl to q,
// its size a share of the box.
func arrowhead(ctl, q [2]float64, size float64) [][2]float64 {
	dx, dy := q[0]-ctl[0], q[1]-ctl[1]
	l := math.Hypot(dx, dy)
	if l == 0 {
		return nil
	}
	ux, uy := dx/l, dy/l
	base := [2]float64{q[0] - ux*size, q[1] - uy*size}
	w := size * 0.55
	return [][2]float64{q, {base[0] - uy*w, base[1] + ux*w}, {base[0] + uy*w, base[1] - ux*w}}
}
