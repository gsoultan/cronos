package h3

import "math"

// Between the sphere and the grids on the icosahedron's faces: a place to the
// face it is on and the cell there, and a cell's corners back to places.
// Ported from H3's faceijk.c and latLng.c.

const (
	epsilon     = 0.0000000000000001
	fltEpsilon  = 1.1920928955078125e-07
	sqrt3over2  = 0.8660254037844386467637231707529361834714
	sqrt7       = 2.6457513110645905905016157536392604257102
	ap7RotRads  = 0.333473172251832115336090755351601070065900389
	res0Gnomic  = 0.38196601125010500003
	numHexVerts = 6
	numPentVert = 5
	// maxBoundary is as many corners as a cell's boundary can have: a
	// hexagon's six, and one where each other edge crosses a face's.
	maxBoundary = 10
)

// The quadrant of a face another lies across, as faceNeighbors indexes it.
const (
	ijQuadrant = 1
	kiQuadrant = 2
	jkQuadrant = 3
)

// faceOrient is how one face's coordinates become its neighbour's.
type faceOrient struct {
	face      int
	translate coordIJK
	ccwRot60  int
}

// overage says whether a coordinate stayed on its face.
type overage int

const (
	noOverage overage = iota
	faceEdge
	newFace
)

// maxDimByCIIres is the largest coordinate a Class II grid's face holds, and
// unitScaleByCIIres its unit in the resolution-0 grid; -1 at Class III ones.
var maxDimByCIIres = [17]int{2, -1, 14, -1, 98, -1, 686, -1, 4802, -1, 33614, -1, 235298, -1, 1647086, -1, 11529602}

var unitScaleByCIIres = [17]int{1, -1, 7, -1, 49, -1, 343, -1, 2401, -1, 16807, -1, 117649, -1, 823543, -1, 5764801}

// classIII is whether a resolution's grid is rotated from the one above it.
func classIII(res int) bool { return res%2 == 1 }

// geoToFaceIJK is the cell holding a place, lat and lng in radians.
func geoToFaceIJK(lat, lng float64, res int) faceIJK {
	face, v := geoToHex2d(lat, lng, res)
	return faceIJK{face: face, coord: coordOf(v)}
}

// geoToHex2d is the face a place is on, and where on that face's plane.
func geoToHex2d(lat, lng float64, res int) (int, vec2) {
	face, sqd := closestFace(lat, lng)
	r := math.Acos(1 - sqd/2)
	if r < epsilon {
		return face, vec2{}
	}
	centre := faceCenterGeo[face]
	theta := posAngle(faceAxesAzRadsCII[face][0] - posAngle(azimuth(centre[0], centre[1], lat, lng)))
	if classIII(res) {
		theta = posAngle(theta - ap7RotRads)
	}
	r = math.Tan(r) / res0Gnomic
	for i := 0; i < res; i++ {
		r *= sqrt7
	}
	return face, vec2{r * math.Cos(theta), r * math.Sin(theta)}
}

// closestFace is the face whose centre is nearest a place, and the squared
// distance to it through the sphere.
func closestFace(lat, lng float64) (int, float64) {
	r := math.Cos(lat)
	x, y, z := math.Cos(lng)*r, math.Sin(lng)*r, math.Sin(lat)
	face, sqd := 0, 5.0
	for f, p := range faceCenterPoint {
		d := (p[0]-x)*(p[0]-x) + (p[1]-y)*(p[1]-y) + (p[2]-z)*(p[2]-z)
		if d < sqd {
			face, sqd = f, d
		}
	}
	return face, sqd
}

// hex2dToGeo is the place a point on a face's plane is, lat and lng in
// radians. A substrate grid is the finer one a cell's corners sit on.
func hex2dToGeo(v vec2, face, res int, substrate bool) (float64, float64) {
	centre := faceCenterGeo[face]
	r := math.Sqrt(v.x*v.x + v.y*v.y)
	if r < epsilon {
		return centre[0], centre[1]
	}
	theta := math.Atan2(v.y, v.x)
	for i := 0; i < res; i++ {
		r /= sqrt7
	}
	if substrate {
		r /= 3
		if classIII(res) {
			r /= sqrt7
		}
	}
	r = math.Atan(r * res0Gnomic)
	if !substrate && classIII(res) {
		theta = posAngle(theta + ap7RotRads)
	}
	theta = posAngle(faceAxesAzRadsCII[face][0] - theta)
	return azDistance(centre[0], centre[1], theta, r)
}

// adjustOverageClassII moves a coordinate that has run off its face onto
// the face it ran onto, and says whether it did — or whether it sits on the
// edge between, which only a substrate grid's corners can.
func adjustOverageClassII(f *faceIJK, res int, pentLeading4, substrate bool) overage {
	maxDim := maxDimByCIIres[res]
	if substrate {
		maxDim *= 3
	}
	c := &f.coord
	sum := c.i + c.j + c.k
	if substrate && sum == maxDim {
		return faceEdge
	}
	if sum <= maxDim {
		return noOverage
	}
	var orient faceOrient
	switch {
	case c.k > 0 && c.j > 0:
		orient = faceNeighbors[f.face][jkQuadrant]
	case c.k > 0:
		orient = faceNeighbors[f.face][kiQuadrant]
		if pentLeading4 {
			// The pentagon's missing sequence: rotate about its centre.
			origin := coordIJK{maxDim, 0, 0}
			*c = c.sub(origin).rotate60cw().add(origin)
		}
	default:
		orient = faceNeighbors[f.face][ijQuadrant]
	}
	f.face = orient.face
	for i := 0; i < orient.ccwRot60; i++ {
		*c = c.rotate60ccw()
	}
	unit := unitScaleByCIIres[res]
	if substrate {
		unit *= 3
	}
	*c = c.add(orient.translate.scale(unit)).normalize()
	if substrate && c.i+c.j+c.k == maxDim {
		return faceEdge
	}
	return newFace
}

// adjustPentVertOverage moves a pentagon's corner onto its face, however
// many faces it takes.
func adjustPentVertOverage(f *faceIJK, res int) overage {
	for {
		if o := adjustOverageClassII(f, res, false, true); o != newFace {
			return o
		}
	}
}

// posAngle is an angle in [0, 2π).
func posAngle(rads float64) float64 {
	t := rads
	if rads < 0 {
		t = rads + 2*math.Pi
	}
	if rads >= 2*math.Pi {
		t -= 2 * math.Pi
	}
	return t
}

// azimuth is the bearing from one place to another, in radians.
func azimuth(lat1, lng1, lat2, lng2 float64) float64 {
	return math.Atan2(math.Cos(lat2)*math.Sin(lng2-lng1),
		math.Cos(lat1)*math.Sin(lat2)-math.Sin(lat1)*math.Cos(lat2)*math.Cos(lng2-lng1))
}

// azDistance is where a bearing and a distance on the unit sphere lead from
// a place, all in radians.
func azDistance(lat1, lng1, az, distance float64) (float64, float64) {
	if distance < epsilon {
		return lat1, lng1
	}
	az = posAngle(az)
	if az < epsilon || math.Abs(az-math.Pi) < epsilon {
		lat := lat1 + distance
		if az >= epsilon {
			lat = lat1 - distance
		}
		return polar(lat, constrainLng(lng1))
	}
	sinlat := clamp(math.Sin(lat1)*math.Cos(distance) + math.Cos(lat1)*math.Sin(distance)*math.Cos(az))
	lat := math.Asin(sinlat)
	if math.Abs(lat-math.Pi/2) < epsilon || math.Abs(lat+math.Pi/2) < epsilon {
		return polar(lat, 0)
	}
	sinlng := clamp(math.Sin(az) * math.Sin(distance) / math.Cos(lat))
	coslng := clamp((math.Cos(distance) - math.Sin(lat1)*math.Sin(lat)) / math.Cos(lat1) / math.Cos(lat))
	return lat, constrainLng(lng1 + math.Atan2(sinlng, coslng))
}

// polar snaps a latitude within epsilon of a pole onto it, where every
// longitude is the same one.
func polar(lat, lng float64) (float64, float64) {
	switch {
	case math.Abs(lat-math.Pi/2) < epsilon:
		return math.Pi / 2, 0
	case math.Abs(lat+math.Pi/2) < epsilon:
		return -math.Pi / 2, 0
	}
	return lat, lng
}

func clamp(v float64) float64 { return max(-1, min(1, v)) }

// constrainLng is a longitude in [-π, π].
func constrainLng(lng float64) float64 {
	for lng > math.Pi {
		lng -= 2 * math.Pi
	}
	for lng < -math.Pi {
		lng += 2 * math.Pi
	}
	return lng
}
