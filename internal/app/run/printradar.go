package run

import (
	"math"

	"github.com/gsoultan/cronos/internal/core/document"
)

// radarReach is the web's radius as a share of the square box, leaving the
// rest for the spokes' names around it.
const radarReach = 0.34

// radar draws a radar as the screen does, in a square box: rings at the
// scale's ticks, a spoke per category with its name outside the web, and each
// series a washed shape with its edge and a dot on each spoke.
func radar(c *document.Chart, b Block) {
	groups := b.Groups
	if len(groups) == 0 {
		groups = []Group{{Label: b.Title, Bars: b.Series}}
	}
	spokes := len(groups[0].Bars)
	if spokes < 3 || b.YAxis == nil {
		c.Note = "A radar needs three categories or more; a bar chart says two better."
		return
	}
	at := func(i int, share float64) [2]float64 {
		a := -math.Pi/2 + float64(i)*2*math.Pi/float64(spokes)
		return [2]float64{0.5 + math.Cos(a)*radarReach*share, 0.5 + math.Sin(a)*radarReach*share}
	}
	web(c, b.YAxis, groups[0].Bars, at)
	for _, g := range groups {
		shape := make([][2]float64, 0, spokes+1)
		for i, v := range g.Bars {
			shape = append(shape, at(i, norm(v.Value, b.YAxis.Min, b.YAxis.Max)))
		}
		c.Marks = append(c.Marks,
			document.Mark{Kind: document.PolyMark, Points: shape, Tone: tone(g.Slot) + "-wash"},
			document.Mark{Kind: document.LineMark, Points: append(shape, shape[0]), Tone: tone(g.Slot), Stroke: 1.2})
		// A share of a box a third the width of a line chart's, so larger.
		for _, p := range shape {
			c.Marks = append(c.Marks, document.Mark{Kind: document.DotMark, X: p[0], Y: p[1], W: 0.011, Tone: tone(g.Slot)})
		}
	}
	c.Keys = keysOf(b.Groups)
}

// web is a radar's rings, spokes and names, and its scale up the first spoke.
func web(c *document.Chart, y *Axis, spokes []Bar, at func(int, float64) [2]float64) {
	for _, t := range y.Ticks {
		if t.At <= 0 {
			continue
		}
		ring := make([][2]float64, 0, len(spokes)+1)
		for i := range spokes {
			ring = append(ring, at(i, t.At))
		}
		c.Marks = append(c.Marks, document.Mark{Kind: document.LineMark, Points: append(ring, ring[0]),
			Tone: "line", Stroke: 0.5},
			text(0.51, 0.5-radarReach*t.At-0.02, t.Label, "muted", document.StartAnchor, false))
	}
	for i, s := range spokes {
		end, out := at(i, 1), at(i, 1.18)
		anchor := ""
		switch {
		case out[0] > 0.52:
			anchor = document.StartAnchor
		case out[0] < 0.48:
			anchor = document.EndAnchor
		}
		c.Marks = append(c.Marks,
			document.Mark{Kind: document.LineMark, Points: [][2]float64{{0.5, 0.5}, end}, Tone: "line", Stroke: 0.5},
			text(out[0], out[1], cut(s.Label, 18), "ink", anchor, false))
	}
}
