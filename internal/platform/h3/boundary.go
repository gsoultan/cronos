package h3

import "math"

// A cell's corners. Each face of the icosahedron is a projection plane of its
// own, so a Class III cell whose edge crosses from one face to the next gains
// a corner where it crosses, and each half is projected from its own face.
// Ported from H3's faceijk.c.

// hexSubstrate and pentSubstrate are an origin cell's corners on the finer
// substrate grid its corners lie on, ccw from the i-axis: Class II's first,
// then Class III's.
var (
	hexCornersCII  = [numHexVerts]coordIJK{{2, 1, 0}, {1, 2, 0}, {0, 2, 1}, {0, 1, 2}, {1, 0, 2}, {2, 0, 1}}
	hexCornersCIII = [numHexVerts]coordIJK{{5, 4, 0}, {1, 5, 0}, {0, 5, 4}, {0, 1, 5}, {4, 0, 5}, {5, 0, 1}}
)

// corners is a cell's corners on the substrate grid, and the resolution that
// grid is at — one finer for a Class III cell. A pentagon has the first five.
func corners(f faceIJK, res, n int) ([numHexVerts]faceIJK, int) {
	verts := hexCornersCII
	if classIII(res) {
		verts = hexCornersCIII
	}
	c := f.coord.downAp3().downAp3r()
	if classIII(res) {
		c = c.downAp7r()
		res++
	}
	var out [numHexVerts]faceIJK
	for v := 0; v < n; v++ {
		out[v] = faceIJK{face: f.face, coord: c.add(verts[v]).normalize()}
	}
	return out, res
}

// faceEdge is the edge of a face a quadrant lies across, on a substrate grid
// at res.
func edgeOf(quadrant, res int) (vec2, vec2) {
	m := float64(maxDimByCIIres[res])
	v0 := vec2{3 * m, 0}
	v1 := vec2{-1.5 * m, 3 * sqrt3over2 * m}
	v2 := vec2{-1.5 * m, -3 * sqrt3over2 * m}
	switch quadrant {
	case ijQuadrant:
		return v0, v1
	case jkQuadrant:
		return v1, v2
	}
	return v2, v0
}

// intersect is where the line through p0 and p1 meets the one through p2 and
// p3.
func intersect(p0, p1, p2, p3 vec2) vec2 {
	s1 := vec2{p1.x - p0.x, p1.y - p0.y}
	s2 := vec2{p3.x - p2.x, p3.y - p2.y}
	t := (s2.x*(p0.y-p2.y) - s2.y*(p0.x-p2.x)) / (-s2.x*s1.y + s1.x*s2.y)
	return vec2{p0.x + t*s1.x, p0.y + t*s1.y}
}

func almostEqual(a, b vec2) bool {
	return math.Abs(a.x-b.x) < fltEpsilon && math.Abs(a.y-b.y) < fltEpsilon
}

// hexBoundary is a hexagon's corners, lat and lng in radians.
func hexBoundary(h faceIJK, res int) [][2]float64 {
	verts, adj := corners(h, res, numHexVerts)
	out := make([][2]float64, 0, maxBoundary)
	lastFace, lastOverage := -1, noOverage
	// One more than the corners: the last edge may cross a face too.
	for vert := 0; vert < numHexVerts+1; vert++ {
		v := vert % numHexVerts
		f := verts[v]
		o := adjustOverageClassII(&f, adj, false, true)
		if classIII(res) && vert > 0 && f.face != lastFace && lastOverage != faceEdge {
			orig0, orig1 := verts[(v+5)%numHexVerts].coord.hex(), verts[v].coord.hex()
			face2 := lastFace
			if lastFace == h.face {
				face2 = f.face
			}
			e0, e1 := edgeOf(adjacentFaceDir[h.face][face2], adj)
			inter := intersect(orig0, orig1, e0, e1)
			// At a corner, both edges lie on one face: no corner to add.
			if !almostEqual(orig0, inter) && !almostEqual(orig1, inter) {
				lat, lng := hex2dToGeo(inter, h.face, adj, true)
				out = append(out, [2]float64{lat, lng})
			}
		}
		if vert < numHexVerts {
			lat, lng := hex2dToGeo(f.coord.hex(), f.face, adj, true)
			out = append(out, [2]float64{lat, lng})
		}
		lastFace, lastOverage = f.face, o
	}
	return out
}

// pentBoundary is a pentagon's corners, lat and lng in radians. Every edge of
// a Class III pentagon crosses a face's.
func pentBoundary(h faceIJK, res int) [][2]float64 {
	verts, adj := corners(h, res, numPentVert)
	out := make([][2]float64, 0, maxBoundary)
	var last faceIJK
	for vert := 0; vert < numPentVert+1; vert++ {
		f := verts[vert%numPentVert]
		adjustPentVertOverage(&f, adj)
		if classIII(res) && vert > 0 {
			orig0 := last.coord.hex()
			orient := faceNeighbors[f.face][adjacentFaceDir[f.face][last.face]]
			tmp := faceIJK{face: orient.face, coord: f.coord}
			for i := 0; i < orient.ccwRot60; i++ {
				tmp.coord = tmp.coord.rotate60ccw()
			}
			tmp.coord = tmp.coord.add(orient.translate.scale(unitScaleByCIIres[adj] * 3)).normalize()
			e0, e1 := edgeOf(adjacentFaceDir[tmp.face][f.face], adj)
			lat, lng := hex2dToGeo(intersect(orig0, tmp.coord.hex(), e0, e1), tmp.face, adj, true)
			out = append(out, [2]float64{lat, lng})
		}
		if vert < numPentVert {
			lat, lng := hex2dToGeo(f.coord.hex(), f.face, adj, true)
			out = append(out, [2]float64{lat, lng})
		}
		last = f
	}
	return out
}
