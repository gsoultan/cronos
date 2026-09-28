package run

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/gsoultan/cronos/internal/core/document"
)

// printTolerance is how much detail a printed map keeps: a thousandth of the
// box, about a tenth of a millimetre at the width a report prints a map.
// The browser's paths are simplified for a screen that can zoom; paper
// cannot, and a coastline's worth of vertices it cannot show only makes the
// document slower to typeset.
const printTolerance = 0.001

/*
printMap draws a map on paper: every layer the browser draws, as marks, in the
order the author listed them — and no basemap.

A map was a line saying to open the report in a browser, because the geometry
reached the viewer as SVG paths and a typesetter wants vertices. The paths are
ours, in a grammar of three letters, so reading the vertices back out of them is
a loop rather than a parser — which is cheaper than carrying every ring twice
through a render that is usually for a browser.

The basemap stays off paper. Its tiles are a third party's, requested by the
reader's browser under that party's terms, and none of those terms is a server
fetching them to print into a document that is then mailed to five thousand
people.
*/
func printMap(c *document.Chart, m *GeoMap) {
	b := m.Bounds
	w, h := b.MaxX-b.MinX, b.MaxY-b.MinY
	if w <= 0 || h <= 0 {
		return
	}
	c.Aspect = w / h
	at := func(x, y float64) [2]float64 { return [2]float64{(x - b.MinX) / w, (y - b.MinY) / h} }
	onPaper(c, m, at)
	// The datasets drawn over it, in order, on the same box.
	for _, ov := range m.Overlays {
		onPaper(c, ov, at)
	}
	c.Keys = mapKeys(m)
	for _, ov := range m.Overlays {
		c.Keys = append(c.Keys, mapKeys(ov)...)
	}
	// On paper as on screen: a map drawn from part of its data says so.
	c.Note = m.Partial
}

// onPaper draws one map's layers as marks, in the order its author listed them.
func onPaper(c *document.Chart, m *GeoMap, at func(x, y float64) [2]float64) {
	keyed := len(m.Keys) > 0
	for _, layer := range m.Layers {
		switch layer {
		case "polygon":
			areas(c, m.Shapes, at, rampTones(m))
		case "hexbin":
			areas(c, m.Hexes, at, rampTones(m))
		case "line":
			strokes(c, m.Lines, at, rampTones(m))
		case "heat":
			for _, p := range m.Markers {
				xy := at(p.X, p.Y)
				c.Marks = append(c.Marks, document.Mark{Kind: document.DotMark,
					X: xy[0], Y: xy[1], W: 0.035, Tone: "ramp-6-wash"})
			}
			cellDots(c, m.Cells, at, layer, keyed)
		case "bubble", "scatter", "cluster":
			pins(c, m.Markers, at, layer == "bubble", keyed)
			cellDots(c, m.Cells, at, layer, keyed)
		case "flow":
			flows(c, m.Arcs, at, keyed)
		}
	}
}

// areas fills polygons, each ring a poly mark and each hole cut back out.
//
// Typst's polygon has no holes, so a lake is painted over its region in paper
// white — which makes the order the whole of it. Every ring of every shape is
// painted in order of how many rings contain it: a region, then the lake in
// it, then the island in the lake, and an exclave of one region sitting in a
// hole of another after that hole. Painting each shape's holes after its own
// fills, and shapes largest first, whited out both the island and the exclave.
// Whether a ring is a hole is its own shape's question; how deep it sits is
// everybody's.
func areas(c *document.Chart, shapes []Shape, at func(x, y float64) [2]float64, tones string) {
	type ring struct {
		pts   [][2]float64
		shape int
		hole  bool
		depth int
		size  float64
	}
	var rings []ring
	for si, s := range shapes {
		own := ringsOf(s.Path, at)
		for i, r := range own {
			rings = append(rings, ring{pts: r, shape: si, hole: nested(r, own, i)%2 == 1})
		}
	}
	all := make([][][2]float64, len(rings))
	boxes := make([]extent, len(rings))
	for i, r := range rings {
		all[i], boxes[i] = r.pts, extentOf(r.pts)
	}
	for i := range rings {
		rings[i].depth = depthOf(i, all, boxes)
		rings[i].size = boxes[i].area()
	}
	slices.SortStableFunc(rings, func(a, b ring) int {
		if a.depth != b.depth {
			return cmp.Compare(a.depth, b.depth)
		}
		return cmp.Compare(b.size, a.size)
	})

	for _, r := range rings {
		if r.hole {
			c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark, Points: r.pts, Tone: "paper"})
			continue
		}
		s := shapes[r.shape]
		c.Marks = append(c.Marks, document.Mark{Kind: document.PolyMark, Points: r.pts,
			Tone: shadeTone(tones, s.Step), Label: s.Label, Value: s.Formatted})
	}
}

// rampTones is the family of tones a map's shades are printed in: the
// sequential ramp's, or a diverging ramp's two hues.
func rampTones(m *GeoMap) string {
	if m.Ramp == "diverging" {
		return "div"
	}
	return "ramp"
}

// shadeTone is the tone for one shade of a ramp.
func shadeTone(tones string, step int) string {
	return fmt.Sprintf("%s-%d", tones, min(max(step, 0), RampSteps-1)+1)
}

// strokes draws each line of the line layer, every leg its own mark.
func strokes(c *document.Chart, lines []Shape, at func(x, y float64) [2]float64, tones string) {
	for _, l := range lines {
		colour := shadeTone(tones, l.Step)
		for _, leg := range legsOf(l.Path, at) {
			c.Marks = append(c.Marks, document.Mark{Kind: document.LineMark, Points: leg,
				Tone: colour, Label: l.Label, Value: l.Formatted})
		}
	}
}

// pins places the markers, sized by value for a bubble layer. A cluster is
// printed as its points: clustering is how a screen copes with a zoom level,
// and a page has one.
func pins(c *document.Chart, markers []Marker, at func(x, y float64) [2]float64, sized, keyed bool) {
	for _, p := range markers {
		xy := at(p.X, p.Y)
		r := 0.006
		if sized {
			r = 0.006 + sqrt(p.Weight)*0.02
		}
		colour, shape := "series-2", ""
		if keyed {
			colour, shape = tone(p.Slot), glyph(p.Slot)
		}
		c.Marks = append(c.Marks, document.Mark{Kind: document.DotMark, X: xy[0], Y: xy[1],
			W: r, Tone: colour, Shape: shape, Label: p.Label, Value: p.Formatted})
	}
}

/*
cellDots prints a large map's cells: a dot per cell, its area the share of the
busiest cell's places it holds — gathered for a page, which cannot zoom, so a
few thousand of them rather than a screen's worth. A heat layer's are washes,
as its places are.
*/
func cellDots(c *document.Chart, cells *Cells, at func(x, y float64) [2]float64, layer string, keyed bool) {
	if cells == nil {
		return
	}
	most := 1
	for _, n := range cells.N {
		most = max(most, n)
	}
	for i, n := range cells.N {
		xy := at(cells.X[i], cells.Y[i])
		share := sqrt(float64(n) / float64(most))
		mark := document.Mark{Kind: document.DotMark, X: xy[0], Y: xy[1],
			W: 0.003 + share*0.012, Tone: "series-2", Value: compact(cells.V[i]),
			Label: places(n)}
		if n == 1 && i < len(cells.L) && cells.L[i] != "" {
			mark.Label = cells.L[i]
		}
		switch {
		case layer == "heat":
			mark.W, mark.Tone = 0.01+share*0.03, "ramp-6-wash"
		case keyed && i < len(cells.S):
			mark.Tone, mark.Shape = tone(cells.S[i]), glyph(cells.S[i])
		}
		c.Marks = append(c.Marks, mark)
	}
}

// flows draws each arc bowed as the browser bows it, sampled into a line.
func flows(c *document.Chart, arcs []Arc, at func(x, y float64) [2]float64, keyed bool) {
	for _, a := range arcs {
		p, q := at(a.X1, a.Y1), at(a.X2, a.Y2)
		dx, dy := q[0]-p[0], q[1]-p[1]
		ctl := [2]float64{(p[0]+q[0])/2 - dy*0.18, (p[1]+q[1])/2 + dx*0.18}
		pts := make([][2]float64, 0, 13)
		for i := range 13 {
			t := float64(i) / 12
			pts = append(pts, [2]float64{
				(1-t)*(1-t)*p[0] + 2*(1-t)*t*ctl[0] + t*t*q[0],
				(1-t)*(1-t)*p[1] + 2*(1-t)*t*ctl[1] + t*t*q[1],
			})
		}
		colour := "series-1"
		if keyed {
			colour = tone(a.Slot)
		}
		c.Marks = append(c.Marks, document.Mark{Kind: document.LineMark, Points: pts,
			Tone: colour, Label: a.Label, Value: a.Formatted})
	}
}

// mapKeys is the printed legend: the ramp's bands, or the categories.
func mapKeys(m *GeoMap) []document.Key {
	var out []document.Key
	if len(m.Shapes)+len(m.Lines)+len(m.Hexes) > 0 {
		for _, l := range m.Legend {
			label := l.From
			if l.To != l.From {
				label += "–" + l.To
			}
			out = append(out, document.Key{Tone: shadeTone(rampTones(m), l.Step), Label: label})
		}
	}
	placed := printsPlaces(m)
	for _, k := range m.Keys {
		key := document.Key{Tone: tone(k.Slot), Label: k.Label}
		if placed {
			key.Shape = glyph(k.Slot)
		}
		out = append(out, key)
	}
	return out
}

// glyph is the shape a category's places are printed in, beside its tone:
// a circle, a square, a triangle, folding past them as the tones fold — the
// viewer's glyphs.ts, on paper.
func glyph(slot int) string {
	switch min(max(slot, 0), PlotSlots-1) {
	case 1:
		return document.SquareShape
	case 2:
		return document.TriangleShape
	}
	return document.CircleShape
}

// printsPlaces reports whether a map prints places, which a category's glyph
// is drawn on — rather than only flows, which are lines.
func printsPlaces(m *GeoMap) bool {
	for _, l := range m.Layers {
		if l == "scatter" || l == "bubble" || l == "cluster" {
			return true
		}
	}
	return false
}

// ringsOf reads the closed subpaths of a path back into rings, projected into
// the box and thinned for paper.
func ringsOf(path string, at func(x, y float64) [2]float64) [][][2]float64 {
	var out [][][2]float64
	for _, sub := range strings.Split(path, "Z") {
		ring := pointsOf(sub, at)
		if len(ring) >= 3 {
			out = append(out, simplify(ring, printTolerance))
		}
	}
	return out
}

// legsOf reads the open subpaths of a line layer's path.
func legsOf(path string, at func(x, y float64) [2]float64) [][][2]float64 {
	var out [][][2]float64
	for _, sub := range strings.Split(path, "M") {
		leg := pointsOf("M"+sub, at)
		if len(leg) >= 2 {
			out = append(out, simplifyLine(leg, printTolerance))
		}
	}
	return out
}

// pointsOf reads one subpath's vertices: "M x yL x yL x y".
func pointsOf(sub string, at func(x, y float64) [2]float64) [][2]float64 {
	var out [][2]float64
	for _, pair := range strings.FieldsFunc(sub, func(r rune) bool { return r == 'M' || r == 'L' }) {
		xy := strings.Fields(pair)
		if len(xy) != 2 {
			continue
		}
		x, errX := strconv.ParseFloat(xy[0], 64)
		y, errY := strconv.ParseFloat(xy[1], 64)
		if errX == nil && errY == nil {
			out = append(out, at(x, y))
		}
	}
	return out
}

// nested counts the other rings of a shape that contain ring i's first
// vertex — odd means ring i is a hole. By containment rather than by winding,
// because GeoJSON's winding rule is one the data does not always follow.
func nested(ring [][2]float64, rings [][][2]float64, i int) int {
	n := 0
	for j, other := range rings {
		if j != i && inside(ring[0], other) {
			n++
		}
	}
	return n
}

// depthOf counts the rings of any shape that contain ring i.
//
// Every ring against every other is quadratic, and a hexbin prints thousands:
// the boxes go first, so the full test runs only where one ring's first
// vertex is already inside another's box.
func depthOf(i int, rings [][][2]float64, boxes []extent) int {
	p := rings[i][0]
	n := 0
	for j, other := range rings {
		if j != i && boxes[j].has(p) && inside(p, other) {
			n++
		}
	}
	return n
}

// extent is the box around a ring.
type extent struct{ minX, minY, maxX, maxY float64 }

func extentOf(pts [][2]float64) extent {
	e := extent{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, p := range pts {
		e.minX, e.minY = math.Min(e.minX, p[0]), math.Min(e.minY, p[1])
		e.maxX, e.maxY = math.Max(e.maxX, p[0]), math.Max(e.maxY, p[1])
	}
	return e
}

func (e extent) has(p [2]float64) bool {
	return p[0] >= e.minX && p[0] <= e.maxX && p[1] >= e.minY && p[1] <= e.maxY
}

func (e extent) area() float64 {
	if e.maxX < e.minX {
		return 0
	}
	return (e.maxX - e.minX) * (e.maxY - e.minY)
}

// inside is the even-odd test: a ray to the right crosses the ring an odd
// number of times from a point inside it.
func inside(p [2]float64, ring [][2]float64) bool {
	in := false
	for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
		a, b := ring[i], ring[j]
		if (a[1] > p[1]) != (b[1] > p[1]) &&
			p[0] < (b[0]-a[0])*(p[1]-a[1])/(b[1]-a[1])+a[0] {
			in = !in
		}
	}
	return in
}
