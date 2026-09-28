package run

import (
	"cmp"
	"math"
	"slices"

	"github.com/gsoultan/cronos/internal/core/document"
)

// nodeWidth is a sankey node's width, as a share of the box.
const nodeWidth = 0.012

// printSankey draws a sankey as the screen does, from the layout the server
// worked out: each source a bar down the left in its colour and each target a
// bar down the right, the bands between them in the source's colour, softened,
// and every node named on its outer side with what flows through it.
func printSankey(c *document.Chart, s *Sankey) {
	if len(s.Nodes) == 0 {
		return
	}
	var left, right []string
	for _, n := range s.Nodes {
		if n.Side == 0 {
			left = append(left, n.Label)
		} else {
			right = append(right, n.Label)
		}
	}
	x0 := nameColumn(left) + 0.1
	x1 := 1 - nameColumn(right) - 0.1 - nodeWidth
	c.Height = rowsHeight(max(len(left), len(right)), 9)
	for _, l := range s.Links {
		c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark,
			Tone: tone(s.Nodes[l.From].Slot) + "-soft", Points: flowBand(x0+nodeWidth, x1, l)})
	}
	for _, n := range s.Nodes {
		x, colour, words := x0, tone(n.Slot), text(x0-0.01, n.Y+n.H/2, n.Label+" · "+n.Formatted, "ink", document.EndAnchor, false)
		if n.Side == 1 {
			x, colour = x1, "neutral"
			words = text(x1+nodeWidth+0.01, n.Y+n.H/2, n.Label+" · "+n.Formatted, "ink", document.StartAnchor, false)
		}
		c.Marks = append(c.Marks, document.Mark{Kind: document.RectMark, X: x, Y: n.Y,
			W: nodeWidth, H: math.Max(n.H, 0.004), Tone: colour}, words)
	}
}

// flowBand is a band from x0 to x1: its top edge a curve from where it leaves
// its source to where it meets its target, and its bottom edge back.
func flowBand(x0, x1 float64, l SankeyLink) [][2]float64 {
	const steps = 16
	edge := func(ya, yb float64, back bool) [][2]float64 {
		pts := make([][2]float64, 0, steps+1)
		for i := 0; i <= steps; i++ {
			t := float64(i) / steps
			if back {
				t = 1 - t
			}
			// The ease a cubic with its handles level at mid-width traces.
			e := t * t * (3 - 2*t)
			pts = append(pts, [2]float64{x0 + (x1-x0)*t, ya + (yb-ya)*e})
		}
		return pts
	}
	return append(edge(l.Y0, l.Y1, false), edge(l.Y0+l.H, l.Y1+l.H, true)...)
}

// sunburst draws parts within parts as the screen does: each series a
// segment of the inner ring as large as its share of the whole, its
// categories around it in its colour, softened, and paper's edge between
// every two. The series are named in the key, and in their segments where
// there is room.
func sunburst(c *document.Chart, b Block) {
	parents, grand := ranked(b.Groups)
	if grand <= 0 {
		return
	}
	from := -0.25
	for _, p := range parents {
		span := p.total / grand
		c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark,
			Points: ringSlice(from, span, 0.34, 0.66), Tone: tone(p.g.Slot), Stroke: 0.8})
		at := from
		for _, child := range p.children {
			part := child.Value / grand
			c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark,
				Points: ringSlice(at, part, 0.68, 1), Tone: tone(p.g.Slot) + "-soft", Stroke: 0.8})
			if part >= 0.06 {
				a := (at + part/2) * 2 * math.Pi
				c.Marks = append(c.Marks, text(0.5+math.Cos(a)*0.42, 0.5+math.Sin(a)*0.42,
					cut(child.Label, 10), "ink", "", false))
			}
			at += part
		}
		if span >= 0.07 {
			a := (from + span/2) * 2 * math.Pi
			c.Marks = append(c.Marks, text(0.5+math.Cos(a)*0.25, 0.5+math.Sin(a)*0.25,
				cut(p.g.Label, 10), "white", "", true))
		}
		from += span
	}
	if len(b.Totals) > 0 {
		// Across most of the hole, as a donut's whole is.
		c.Marks = append(c.Marks, headline(0.48, b.Totals[0].Formatted, 13, 0.34*0.78),
			text(0.5, 0.57, b.Totals[0].Label, "muted", "", false))
	}
	c.Keys = keysOf(b.Groups)
}

// parent is one series of a sunburst, with its total and its categories.
type parent struct {
	g        Group
	total    float64
	children []Bar
}

// ranked is a sunburst's series, largest first, each with the categories it
// holds, largest first — and the whole. Nothing or less is not a part.
func ranked(groups []Group) ([]parent, float64) {
	var out []parent
	grand := 0.0
	for _, g := range groups {
		p := parent{g: g}
		for _, b := range g.Bars {
			if b.Value > 0 {
				p.total += b.Value
				p.children = append(p.children, b)
			}
		}
		slices.SortStableFunc(p.children, func(a, b Bar) int { return cmp.Compare(b.Value, a.Value) })
		if p.total > 0 {
			out = append(out, p)
			grand += p.total
		}
	}
	slices.SortStableFunc(out, func(a, b parent) int { return cmp.Compare(b.total, a.total) })
	return out, grand
}

// ringSlice is a segment of a ring in the unit box, from `from` turns for
// `span` turns, between inner and outer — each a share of the box's half.
func ringSlice(from, span, inner, outer float64) [][2]float64 {
	steps := maxInt(int(math.Abs(span)*sliceSegments), 2)
	at := func(turn, r float64) [2]float64 {
		a := turn * 2 * math.Pi
		return [2]float64{0.5 + math.Cos(a)*r*0.5, 0.5 + math.Sin(a)*r*0.5}
	}
	pts := make([][2]float64, 0, steps*2+2)
	for i := 0; i <= steps; i++ {
		pts = append(pts, at(from+span*float64(i)/float64(steps), outer))
	}
	for i := steps; i >= 0; i-- {
		pts = append(pts, at(from+span*float64(i)/float64(steps), inner))
	}
	return pts
}
