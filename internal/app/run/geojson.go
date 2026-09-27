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

// geoKind is what a geometry draws as: an area to fill or a line to stroke.
type geoKind int

const (
	areaGeometry geoKind = iota
	lineGeometry
)

// geoJSON is as much of the format as the geometry layers read.
//
// Not a full GeoJSON model. The polygon layer shades areas and the line layer
// strokes lines, so those are what it parses; a Point in a geometry column is
// an author pointing a layer at the wrong field — points are lat and lon — and
// saying so beats rendering nothing.
type geoJSON struct {
	Type string `json:"type"`
	// Coordinates is left raw because its depth depends on Type — two levels
	// for a LineString, three for a Polygon and four for a MultiPolygon — and
	// decoding it twice beats an any-typed tree walked with type assertions.
	Coordinates json.RawMessage `json:"coordinates"`
	// Geometry carries the shape when the column holds a whole Feature, which
	// is what ST_AsGeoJSON returns for a row rather than for a column.
	Geometry *geoJSON `json:"geometry"`
}

// geoPath converts one GeoJSON value to an SVG path in Web Mercator world
// units, growing box to contain it, and says whether it is an area or a line.
//
// An SVG path rather than coordinates, because the alternative is shipping
// rings and a projection to every one of an ISV's end users — the same
// argument that keeps currency formatting on the server. The viewer sets a
// viewBox and draws the string.
func geoPath(raw string, tolerance float64, box *Bounds) (string, geoKind, error) {
	var g geoJSON
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return "", 0, fmt.Errorf("%w: geometry is not GeoJSON: %s", ErrNotRenderable, err)
	}
	if g.Type == "Feature" && g.Geometry != nil {
		g = *g.Geometry
	}

	parts, kind, err := g.parts()
	if err != nil {
		return "", 0, err
	}
	var b strings.Builder
	seen := 0
	for _, part := range parts {
		if seen += len(part); seen > VertexLimit {
			return "", 0, fmt.Errorf("%w: geometry has more than %d points",
				ErrNotRenderable, VertexLimit)
		}
		appendPart(&b, part, kind, tolerance, box)
	}
	return b.String(), kind, nil
}

// parts flattens a geometry to the rings or lines it draws, and says which.
//
// Every ring of every polygon becomes one subpath, including the inner rings
// that are holes. The viewer fills even-odd, so a lake inside a country is a
// hole rather than a second shape drawn over the top, whichever way the data
// wound it — GeoJSON asks for opposite windings, and data does not always
// keep to it.
func (g geoJSON) parts() ([][][2]float64, geoKind, error) {
	switch g.Type {
	case "MultiPolygon":
		var polys [][][][2]float64
		if err := json.Unmarshal(g.Coordinates, &polys); err != nil {
			return nil, 0, err
		}
		var out [][][2]float64
		for _, p := range polys {
			out = append(out, p...)
		}
		return out, areaGeometry, nil
	case "Polygon", "MultiLineString":
		var rings [][][2]float64
		if err := json.Unmarshal(g.Coordinates, &rings); err != nil {
			return nil, 0, err
		}
		if g.Type == "Polygon" {
			return rings, areaGeometry, nil
		}
		return rings, lineGeometry, nil
	case "LineString":
		var line [][2]float64
		if err := json.Unmarshal(g.Coordinates, &line); err != nil {
			return nil, 0, err
		}
		return [][][2]float64{line}, lineGeometry, nil
	}
	return nil, 0, fmt.Errorf("%w: a geometry layer draws areas and lines, and this "+
		"geometry is a %q — put points in lat and lon", ErrNotRenderable, g.Type)
}

// appendPart writes one ring as a closed subpath, or one line as an open one.
func appendPart(b *strings.Builder, part [][2]float64, kind geoKind, tolerance float64, box *Bounds) {
	least := 3
	if kind == lineGeometry {
		least = 2
	}
	if len(part) < least {
		return
	}
	pts := make([][2]float64, 0, len(part))
	for _, c := range part {
		if !finite(c[0], c[1]) {
			continue
		}
		x, y := project(c[0], c[1])
		pts = append(pts, [2]float64{x, y})
		box.add(x, y)
	}
	if len(pts) < least {
		return
	}

	if kind == lineGeometry {
		pts = simplifyLine(pts, tolerance)
	} else {
		pts = simplify(pts, tolerance)
	}
	for i, p := range pts {
		if i == 0 {
			b.WriteByte('M')
		} else {
			b.WriteByte('L')
		}
		b.WriteString(coord(p[0]))
		b.WriteByte(' ')
		b.WriteString(coord(p[1]))
	}
	if kind == areaGeometry {
		b.WriteByte('Z')
	}
}

// coordScale is the grid a world-unit coordinate is rounded onto.
//
// Eight decimals is about 40 centimetres at the equator: under a pixel at the
// deepest zoom any tile source serves. It was six — 40 metres — which was
// finer than a map fitted to its data could show and coarser than a map a
// reader can zoom into, where a district boundary snapped to a 40-metre grid
// walks off its own streets by a dozen pixels.
const coordScale = 1e8

// coord formats one world-unit coordinate, rounded and without trailing zeros.
//
// -1 precision after rounding rather than a fixed count: FormatFloat with a
// fixed count pads "0.5" out to "0.50000000", and eight zeros per coordinate
// across a continent's worth of vertices is a measurable part of the payload.
func coord(v float64) string {
	return strconv.FormatFloat(math.Round(v*coordScale)/coordScale, 'f', -1, 64)
}
