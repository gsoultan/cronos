package run

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// VertexLimit caps the points one shape contributes to a payload.
//
// The geometry is a value from our customer's database, so it is the shape of
// the data an attacker in a multi-tenant product has the most control over.
// A single MultiPolygon of ten million vertices is a well-formed GeoJSON
// document, a several-hundred-megabyte response, and a browser tab that never
// comes back. Simplification usually gets there first; this is the bound that
// does not depend on the tolerance an author chose.
const VertexLimit = 20_000

// geoJSON is as much of the format as a choropleth reads.
//
// Not a full GeoJSON model. A polygon layer shades areas, so areas are what it
// parses; a Point or a LineString in a geometry column is an author pointing
// the layer at the wrong field, and saying so beats rendering nothing.
type geoJSON struct {
	Type string `json:"type"`
	// Coordinates is left raw because its depth depends on Type — three levels
	// for a Polygon and four for a MultiPolygon — and decoding it twice beats
	// an any-typed tree walked with type assertions.
	Coordinates json.RawMessage `json:"coordinates"`
	// Geometry carries the shape when the column holds a whole Feature, which
	// is what ST_AsGeoJSON returns for a row rather than for a column.
	Geometry *geoJSON `json:"geometry"`
}

// geoPath converts one GeoJSON value to an SVG path in Web Mercator world
// units, growing box to contain it.
//
// An SVG path rather than coordinates, because the alternative is shipping
// rings and a projection to every one of an ISV's end users — the same
// argument that keeps currency formatting on the server. The viewer sets a
// viewBox and draws the string.
func geoPath(raw string, tolerance float64, box *Bounds) (string, error) {
	var g geoJSON
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return "", fmt.Errorf("%w: geometry is not GeoJSON: %s", ErrNotRenderable, err)
	}
	if g.Type == "Feature" && g.Geometry != nil {
		g = *g.Geometry
	}

	polys, err := g.polygons()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	seen := 0
	for _, poly := range polys {
		for _, ring := range poly {
			if seen += len(ring); seen > VertexLimit {
				return "", fmt.Errorf("%w: geometry has more than %d points",
					ErrNotRenderable, VertexLimit)
			}
			appendRing(&b, ring, tolerance, box)
		}
	}
	return b.String(), nil
}

// polygons normalises Polygon and MultiPolygon to the same nesting.
func (g geoJSON) polygons() ([][][][2]float64, error) {
	switch g.Type {
	case "MultiPolygon":
		var out [][][][2]float64
		return out, json.Unmarshal(g.Coordinates, &out)
	case "Polygon":
		var one [][][2]float64
		if err := json.Unmarshal(g.Coordinates, &one); err != nil {
			return nil, err
		}
		return [][][][2]float64{one}, nil
	}
	return nil, fmt.Errorf("%w: a polygon layer shades areas, and this geometry is a %q",
		ErrNotRenderable, g.Type)
}

// appendRing writes one ring as a closed subpath.
//
// Every ring of every polygon goes into one path string, including the inner
// rings that are holes. SVG's default even-odd-ish `fill-rule: nonzero` with
// GeoJSON's opposite winding for holes cuts them out on its own, so a lake
// inside a country is a hole rather than a second shape drawn over the top.
func appendRing(b *strings.Builder, ring [][2]float64, tolerance float64, box *Bounds) {
	if len(ring) < 3 {
		return
	}
	pts := make([][2]float64, 0, len(ring))
	for _, c := range ring {
		if !finite(c[0], c[1]) {
			continue
		}
		x, y := project(c[0], c[1])
		pts = append(pts, [2]float64{x, y})
		box.add(x, y)
	}
	if len(pts) < 3 {
		return
	}

	for i, p := range simplify(pts, tolerance) {
		if i == 0 {
			b.WriteByte('M')
		} else {
			b.WriteByte('L')
		}
		b.WriteString(coord(p[0]))
		b.WriteByte(' ')
		b.WriteString(coord(p[1]))
	}
	b.WriteByte('Z')
}

// coordScale is the grid a world-unit coordinate is rounded onto.
//
// Six decimals is about 40 metres at the equator, which is finer than a report
// rendered a few hundred pixels across can show and small enough that the
// digits past it are pure payload.
const coordScale = 1e6

// coord formats one world-unit coordinate, rounded and without trailing zeros.
//
// -1 precision after rounding rather than a fixed 6: FormatFloat with a fixed
// count pads "0.5" out to "0.500000", and six zeros per coordinate across a
// continent's worth of vertices is a measurable part of the payload.
func coord(v float64) string {
	return strconv.FormatFloat(math.Round(v*coordScale)/coordScale, 'f', -1, 64)
}
