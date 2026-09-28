/*
Package h3 is Uber's H3 grid — hexagons in a hierarchy that tiles the whole
sphere, twelve pentagons aside — for maps that fold places into it, or draw
cells a warehouse has already indexed.

The two things cronos asks of H3 — which cell holds a place, and where a
cell's corners are — ported from H3 4.1's C library to Go, with the tables
generated from its source. Ported rather than linked: the library's cgo
binding would end cgo-free builds, and the Go transpilation of it hands the
collector pointers it cannot vouch for, which the race detector stops. It
agrees with H3 cell for cell and corner for corner — h3_test.go holds the
proof. H3 is Copyright Uber Technologies, Inc., under the Apache License 2.0
in LICENSE; this package is a derivative of it.
*/
package h3

import (
	"math"
	"strconv"
)

// MaxResolution is H3's finest: cells about half a metre across.
const MaxResolution = maxRes

// Cell is the index of the cell at resolution res holding a place, latitude
// and longitude in degrees — or 0, for a resolution H3 does not have or a
// place that is not on the globe. H3's own arithmetic would index latitude 91
// by carrying it over the pole; a row that says 91 is a row that is wrong.
func Cell(lat, lon float64, res int) uint64 {
	if res < 0 || res > maxRes || !(lat >= -90 && lat <= 90) || !(lon >= -180 && lon <= 180) {
		return 0
	}
	return faceIJKToH3(geoToFaceIJK(lat*(math.Pi/180), lon*(math.Pi/180), res), res)
}

// Parse reads a cell id as warehouses write one — fifteen hex digits,
// 871969c9bffffff — and reports whether it names a cell.
func Parse(s string) (uint64, bool) {
	h, err := strconv.ParseUint(s, 16, 64)
	if err != nil || !valid(h) {
		return 0, false
	}
	return h, true
}

// Valid reports whether h is a cell index — for one a warehouse stores as a
// number rather than as text.
func Valid(h uint64) bool { return valid(h) }

// String is a cell's id, as Parse reads it.
func String(h uint64) string { return strconv.FormatUint(h, 16) }

// Resolution is how fine a cell is, 0 to MaxResolution.
func Resolution(h uint64) int { return resolution(h) }

// Parent is the cell at resolution res that h lies in — h itself at its own
// resolution or a finer one, which no cell has a single one of. An H3 index
// holds its ancestry in its digits, so a parent is the index cut short.
func Parent(h uint64, res int) uint64 {
	if res < 0 || res >= resolution(h) {
		return h
	}
	h = h&^(15<<resOffset) | uint64(res)<<resOffset
	for r := res + 1; r <= maxRes; r++ {
		h = withDigit(h, r, invalidDigit)
	}
	return h
}

// Boundary is a cell's corners in degrees, latitude then longitude, in order
// around it: six for a hexagon and five for a pentagon, and one more for
// each edge that crosses from one face of the icosahedron to the next.
// Nothing, for an index that is not a cell.
func Boundary(h uint64) [][2]float64 {
	if !valid(h) {
		return nil
	}
	f := h3ToFaceIJK(h)
	var rad [][2]float64
	if pentagon(h) {
		rad = pentBoundary(f, resolution(h))
	} else {
		rad = hexBoundary(f, resolution(h))
	}
	for i, p := range rad {
		rad[i] = [2]float64{p[0] * (180 / math.Pi), p[1] * (180 / math.Pi)}
	}
	return rad
}

// edgesKm are the average edge of a cell at each resolution, in kilometres,
// from H3's own table.
var edgesKm = [maxRes + 1]float64{
	1281.256011, 483.0568391, 182.5129565, 68.97922179, 26.07175968, 9.854090990,
	3.724532667, 1.406475763, 0.531414010, 0.200786148, 0.075863783, 0.028663897,
	0.010830188, 0.004092010, 0.001546100, 0.000584169,
}

// EdgeKm is the average edge of a cell at res, for sizing a grid to data.
func EdgeKm(res int) float64 {
	return edgesKm[max(0, min(res, maxRes))]
}
