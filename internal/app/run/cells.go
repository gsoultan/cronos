package run

import (
	"sort"
	"strconv"
)

/*
Cells are a large map's places gathered into a grid: a dot, a bubble, a splat
of heat or a member of a cluster per cell, for the layers that draw points.

Parallel lists rather than a list of objects. Fifty thousand cells as objects
would be fifty thousand copies of seven key names, which is most of the bytes;
as lists they are numbers and commas. A cell holding one place is that place —
its own position, its own label and value — which at the depth a reader zooms to
is nearly every cell, so the same payload serves the overview and the street.
*/
type Cells struct {
	// Size is a cell's edge in world units. A viewer asks again for a view
	// once it is looking closely enough that a cell is several pixels wide.
	Size float64 `json:"size"`
	// X and Y are the middle of each cell's places, in world units.
	X []float64 `json:"x"`
	Y []float64 `json:"y"`
	// N is how many places each cell holds.
	N []int `json:"n"`
	// V is the measure folded over them: its own aggregate again, or their
	// average when the measure is itself an average — see Mean.
	V []float64 `json:"v"`
	// Z is the bubble's size measure, folded the same way.
	Z []float64 `json:"z,omitempty"`
	// S is each cell's category slot, when points are coloured by one.
	S []int `json:"s,omitempty"`
	// L is a lone place's label, and empty for a cell of several.
	L []string `json:"l,omitempty"`
	// Mean says V is an average across each cell's places rather than a
	// total, which is what a tooltip has to say about it.
	Mean bool `json:"mean,omitempty"`

	// cut is whether the view had more cells than a map draws.
	cut bool
}

// sort orders the cells north to south and west to east, so the same view is
// the same payload however the database grouped it.
func (c *Cells) sort() {
	idx := make([]int, len(c.X))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		i, j := idx[a], idx[b]
		if c.Y[i] != c.Y[j] {
			return c.Y[i] < c.Y[j]
		}
		return c.X[i] < c.X[j]
	})
	c.X, c.Y, c.N, c.V = permute(c.X, idx), permute(c.Y, idx), permute(c.N, idx), permute(c.V, idx)
	c.Z, c.S, c.L = permute(c.Z, idx), permute(c.S, idx), permute(c.L, idx)
}

// permute reorders s by idx; an absent list stays absent.
func permute[T any](s []T, idx []int) []T {
	if len(s) == 0 {
		return s
	}
	out := make([]T, len(idx))
	for i, j := range idx {
		out[i] = s[j]
	}
	return out
}

// MarshalJSON writes positions to the precision a screen can use and values
// to the precision they were stored at, rather than seventeen digits of each.
func (c Cells) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, 64+len(c.X)*40)
	b = append(b, `{"size":`...)
	b = strconv.AppendFloat(b, c.Size, 'g', -1, 64)
	b = floats(append(b, `,"x":`...), c.X, 'f', 9)
	b = floats(append(b, `,"y":`...), c.Y, 'f', 9)
	b = append(b, `,"n":[`...)
	for i, n := range c.N {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendInt(b, int64(n), 10)
	}
	b = floats(append(b, `],"v":`...), c.V, 'g', 7)
	if len(c.Z) > 0 {
		b = floats(append(b, `,"z":`...), c.Z, 'g', 7)
	}
	if len(c.S) > 0 {
		b = append(b, `,"s":[`...)
		for i, s := range c.S {
			if i > 0 {
				b = append(b, ',')
			}
			b = strconv.AppendInt(b, int64(s), 10)
		}
		b = append(b, ']')
	}
	if len(c.L) > 0 {
		b = append(b, `,"l":[`...)
		for i, l := range c.L {
			if i > 0 {
				b = append(b, ',')
			}
			b = strconv.AppendQuote(b, l)
		}
		b = append(b, ']')
	}
	if c.Mean {
		b = append(b, `,"mean":true`...)
	}
	return append(b, '}'), nil
}

// floats appends a JSON array of numbers. Nine decimals of a world unit is a
// few centimetres at the equator; seven significant figures of a measure is
// more than any legend shows.
func floats(b []byte, vs []float64, format byte, prec int) []byte {
	b = append(b, '[')
	for i, v := range vs {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendFloat(b, v, format, prec, 64)
	}
	return append(b, ']')
}
