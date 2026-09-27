package run

import (
	"math"
	"sort"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// DefaultHexes is how many hexagons span the data when the author gave no
// width: enough that a city's neighbourhoods separate, few enough that each
// hexagon still holds more than one point.
const DefaultHexes = 24

// earthKm is the equator's length, which is one world unit in kilometres.
const earthKm = 40_075.017

// hexCell is one hexagon of the grid, in axial coordinates.
type hexCell struct{ q, r int }

// hexSum is what a hexagon has folded so far.
type hexSum struct {
	value float64
	count int
}

/*
hexbin folds the map's points into a hexagonal grid: the hexbin layer.

Hexagons rather than squares, because every neighbour of a hexagon is the same
distance away — a hot spot reads as a blob rather than a plus sign. On the
server, like the projection, because the fold needs every point: a viewer
streaming them would not know a hexagon's total until the last one arrived,
and would be doing arithmetic on our customer's customer's main thread that
this machine has already done.

The grid is regular in Web Mercator rather than on the ground, which is what
every web map's hexbin is. Across a city or a country the difference is a few
percent; across a continent a northern hexagon covers less ground than a
southern one — see hexRadius for where the width an author asks for holds.
*/
func hexbin(markers []Marker, radius float64, fold definition.Fold, box *Bounds) []Shape {
	sums := map[hexCell]*hexSum{}
	for _, m := range markers {
		c := cellOf(m.X, m.Y, radius)
		if s, ok := sums[c]; ok {
			s.value = refold(fold, s.value, m.Value)
			s.count++
			continue
		}
		sums[c] = &hexSum{value: m.Value, count: 1}
	}

	// Ordered, because a map's iteration order is random and a payload that
	// lists the same hexagons differently on every render cannot be compared,
	// cached or diffed.
	cells := make([]hexCell, 0, len(sums))
	for c := range sums {
		cells = append(cells, c)
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].r != cells[j].r {
			return cells[i].r < cells[j].r
		}
		return cells[i].q < cells[j].q
	})

	out := make([]Shape, 0, len(cells))
	for _, c := range cells {
		s := sums[c]
		out = append(out, Shape{
			Label: places(s.count), Path: hexPath(c, radius, box),
			Value: s.value, Formatted: compact(s.value),
		})
	}
	return out
}

// hexRadius is the corner-to-centre size of a hexagon, in world units.
//
// From the author's width when there is one, flat side to flat side;
// otherwise sized so DefaultHexes of them span the points.
//
// A width in kilometres is exact at one latitude only: the grid is regular in
// Web Mercator, which stretches a kilometre by 1/cos(latitude). It was
// measured at the middle of the data, which a filter moves — so filtering by
// carrier resized the hexagons and slid the grid under them, the one thing a
// width in kilometres was there to prevent. The stretch there is rounded to a
// step of five percent instead: a filter leaves the grid exactly where it was
// unless it moves the data far enough to change the stretch by that much, and
// the width is right to within two and a half percent wherever the data is.
func hexRadius(km float64, points *Bounds) float64 {
	if points.empty() {
		return 1.0 / 256 / DefaultHexes
	}
	if km > 0 {
		width := km / earthKm * stretch(latitude((points.MinY+points.MaxY)/2))
		return width / math.Sqrt(3)
	}
	span := max(points.MaxX-points.MinX, points.MaxY-points.MinY, 1.0/256)
	return span / DefaultHexes / math.Sqrt(3)
}

// cellOf is the hexagon containing (x, y), for pointy-top hexagons of the
// given radius: axial coordinates, rounded through cube coordinates so a
// point on an edge lands in exactly one hexagon.
func cellOf(x, y, radius float64) hexCell {
	q := (math.Sqrt(3)/3*x - y/3) / radius
	r := (2.0 / 3 * y) / radius

	cx, cz := math.Round(q), math.Round(r)
	cy := math.Round(-q - r)
	dx, dy, dz := math.Abs(cx-q), math.Abs(cy-(-q-r)), math.Abs(cz-r)
	switch {
	case dx > dy && dx > dz:
		cx = -cy - cz
	case dy <= dz:
		cz = -cx - cy
	}
	return hexCell{q: int(cx), r: int(cz)}
}

// hexPath is one hexagon as a closed SVG subpath, growing box to hold it.
func hexPath(c hexCell, radius float64, box *Bounds) string {
	cx := radius * math.Sqrt(3) * (float64(c.q) + float64(c.r)/2)
	cy := radius * 1.5 * float64(c.r)

	var b strings.Builder
	for i := range 6 {
		a := math.Pi / 180 * float64(60*i-30)
		x, y := cx+radius*math.Cos(a), cy+radius*math.Sin(a)
		box.add(clamp01(x), clamp01(y))
		if i == 0 {
			b.WriteByte('M')
		} else {
			b.WriteByte('L')
		}
		b.WriteString(coord(x))
		b.WriteByte(' ')
		b.WriteString(coord(y))
	}
	b.WriteByte('Z')
	return b.String()
}

// stretch is Web Mercator's scale factor at lat, to the nearest power of 1.05.
func stretch(lat float64) float64 {
	sec := 1 / math.Cos(lat*math.Pi/180)
	return math.Pow(1.05, math.Round(math.Log(sec)/math.Log(1.05)))
}

// latitude is project's inverse for y alone.
func latitude(y float64) float64 {
	return math.Atan(math.Sinh(math.Pi*(1-2*y))) * 180 / math.Pi
}

// places counts the points in a hexagon, for its tooltip.
func places(n int) string {
	if n == 1 {
		return "1 location"
	}
	return group(float64(n), 0) + " locations"
}
