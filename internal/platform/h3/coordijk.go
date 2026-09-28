package h3

import "math"

// The hexagon grid in integer coordinates: three axes 120° apart, i, j and
// k, of which a normalised coordinate uses at most two. Ported from H3's
// coordijk.c; the names are H3's, so the two can be read side by side.

// coordIJK is a cell of one face's grid, ijk+ normalised: no component is
// negative, and at least one is zero.
type coordIJK struct{ i, j, k int }

// faceIJK is a cell on one of the icosahedron's twenty faces.
type faceIJK struct {
	face  int
	coord coordIJK
}

// vec2 is a point on a face's plane, in the units of the grid it is on.
type vec2 struct{ x, y float64 }

// Directions, as index digits: the axis, or pair of axes, a child lies along
// from its parent's centre.
const (
	centerDigit = 0
	kAxesDigit  = 1
	jAxesDigit  = 2
	jkAxesDigit = 3
	iAxesDigit  = 4
	ikAxesDigit = 5
	ijAxesDigit = 6
	// invalidDigit fills an index's digits past its resolution.
	invalidDigit = 7
)

// unitVecs is the unit coordinate along each direction.
var unitVecs = [7]coordIJK{{0, 0, 0}, {0, 0, 1}, {0, 1, 0}, {0, 1, 1}, {1, 0, 0}, {1, 0, 1}, {1, 1, 0}}

func (c coordIJK) add(o coordIJK) coordIJK { return coordIJK{c.i + o.i, c.j + o.j, c.k + o.k} }
func (c coordIJK) sub(o coordIJK) coordIJK { return coordIJK{c.i - o.i, c.j - o.j, c.k - o.k} }
func (c coordIJK) scale(f int) coordIJK    { return coordIJK{c.i * f, c.j * f, c.k * f} }

// normalize removes negative components, then the smallest one.
func (c coordIJK) normalize() coordIJK {
	if c.i < 0 {
		c.j -= c.i
		c.k -= c.i
		c.i = 0
	}
	if c.j < 0 {
		c.i -= c.j
		c.k -= c.j
		c.j = 0
	}
	if c.k < 0 {
		c.i -= c.k
		c.j -= c.k
		c.k = 0
	}
	if m := min(c.i, c.j, c.k); m > 0 {
		c.i -= m
		c.j -= m
		c.k -= m
	}
	return c
}

// digit is the direction a unit coordinate points, or invalidDigit.
func (c coordIJK) digit() int {
	n := c.normalize()
	for d, u := range unitVecs {
		if n == u {
			return d
		}
	}
	return invalidDigit
}

// combine is i·iv + j·jv + k·kv, normalised: every aperture step and rotation
// below is one of these with its own three unit vectors.
func (c coordIJK) combine(iv, jv, kv coordIJK) coordIJK {
	return iv.scale(c.i).add(jv.scale(c.j)).add(kv.scale(c.k)).normalize()
}

// upAp7 is the parent coordinate in the next coarser, counter-clockwise
// aperture-7 grid; upAp7r the clockwise one.
func (c coordIJK) upAp7() coordIJK {
	i, j := c.i-c.k, c.j-c.k
	return coordIJK{round(float64(3*i-j) / 7), round(float64(i+2*j) / 7), 0}.normalize()
}

func (c coordIJK) upAp7r() coordIJK {
	i, j := c.i-c.k, c.j-c.k
	return coordIJK{round(float64(2*i+j) / 7), round(float64(3*j-i) / 7), 0}.normalize()
}

// round is C's lround: halves away from zero.
func round(v float64) int { return int(math.Round(v)) }

// downAp7 and downAp7r are the centre of a cell in the next finer aperture-7
// grid; downAp3 and downAp3r in the aperture-3 substrate its vertices are on.
func (c coordIJK) downAp7() coordIJK {
	return c.combine(coordIJK{3, 0, 1}, coordIJK{1, 3, 0}, coordIJK{0, 1, 3})
}
func (c coordIJK) downAp7r() coordIJK {
	return c.combine(coordIJK{3, 1, 0}, coordIJK{0, 3, 1}, coordIJK{1, 0, 3})
}
func (c coordIJK) downAp3() coordIJK {
	return c.combine(coordIJK{2, 0, 1}, coordIJK{1, 2, 0}, coordIJK{0, 1, 2})
}
func (c coordIJK) downAp3r() coordIJK {
	return c.combine(coordIJK{2, 1, 0}, coordIJK{0, 2, 1}, coordIJK{1, 0, 2})
}

// rotate60ccw and rotate60cw turn a coordinate about the origin.
func (c coordIJK) rotate60ccw() coordIJK {
	return c.combine(coordIJK{1, 1, 0}, coordIJK{0, 1, 1}, coordIJK{1, 0, 1})
}

func (c coordIJK) rotate60cw() coordIJK {
	return c.combine(coordIJK{1, 0, 1}, coordIJK{1, 1, 0}, coordIJK{0, 1, 1})
}

// neighbor is the adjacent cell in a direction.
func (c coordIJK) neighbor(digit int) coordIJK {
	if digit > centerDigit && digit < invalidDigit {
		return c.add(unitVecs[digit]).normalize()
	}
	return c
}

// rotateDigit60ccw and rotateDigit60cw turn a direction.
func rotateDigit60ccw(d int) int {
	switch d {
	case kAxesDigit:
		return ikAxesDigit
	case ikAxesDigit:
		return iAxesDigit
	case iAxesDigit:
		return ijAxesDigit
	case ijAxesDigit:
		return jAxesDigit
	case jAxesDigit:
		return jkAxesDigit
	case jkAxesDigit:
		return kAxesDigit
	}
	return d
}

func rotateDigit60cw(d int) int {
	switch d {
	case kAxesDigit:
		return jkAxesDigit
	case jkAxesDigit:
		return jAxesDigit
	case jAxesDigit:
		return ijAxesDigit
	case ijAxesDigit:
		return iAxesDigit
	case iAxesDigit:
		return ikAxesDigit
	case ikAxesDigit:
		return kAxesDigit
	}
	return d
}

// hex returns a coordinate's position on its face's plane.
func (c coordIJK) hex() vec2 {
	i, j := c.i-c.k, c.j-c.k
	return vec2{float64(i) - 0.5*float64(j), float64(j) * sqrt3over2}
}

// coordOf quantises a point on a face's plane to the cell holding it.
func coordOf(v vec2) coordIJK {
	a1, a2 := math.Abs(v.x), math.Abs(v.y)
	x2 := a2 / sqrt3over2
	x1 := a1 + x2/2
	m1, m2 := int(x1), int(x2)
	r1, r2 := x1-float64(m1), x2-float64(m2)
	var h coordIJK
	switch {
	case r1 < 0.5 && r1 < 1.0/3:
		h.i, h.j = m1, m2
		if r2 >= (1+r1)/2 {
			h.j = m2 + 1
		}
	case r1 < 0.5:
		h.i, h.j = m1, m2
		if r2 >= 1-r1 {
			h.j = m2 + 1
		}
		if 1-r1 <= r2 && r2 < 2*r1 {
			h.i = m1 + 1
		}
	case r1 < 2.0/3:
		h.i, h.j = m1+1, m2
		if r2 >= 1-r1 {
			h.j = m2 + 1
		}
		if 2*r1-1 < r2 && r2 < 1-r1 {
			h.i = m1
		}
	default:
		h.i, h.j = m1+1, m2
		if r2 >= r1/2 {
			h.j = m2 + 1
		}
	}
	return fold(h, v)
}

// fold reflects a quantised coordinate back across the axes the point's
// signs put it on the far side of.
func fold(h coordIJK, v vec2) coordIJK {
	if v.x < 0 {
		if h.j%2 == 0 {
			axis := h.j / 2
			h.i -= 2 * (h.i - axis)
		} else {
			axis := (h.j + 1) / 2
			h.i -= 2*(h.i-axis) + 1
		}
	}
	if v.y < 0 {
		h.i -= (2*h.j + 1) / 2
		h.j = -h.j
	}
	return h.normalize()
}
