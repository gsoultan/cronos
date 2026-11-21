package run

import (
	"math"

	"github.com/gsoultan/cronos/internal/core/document"
)

// printDumbbell draws a dumbbell as the screen does: a row per category, its
// name on the left, a grey dot where it ran from and a coloured one where it
// ran to joined by a line, both read along one scale under the box, and the
// value it ended at with the change after the rows.
func printDumbbell(c *document.Chart, b Block) {
	if len(b.Dumbbells) == 0 || b.XAxis == nil {
		return
	}
	labels := make([]string, len(b.Dumbbells))
	words := make([]string, len(b.Dumbbells))
	longest := 0
	for i, d := range b.Dumbbells {
		labels[i] = d.Label
		words[i] = d.ToSaid + " (" + d.Change + ")"
		longest = max(longest, len([]rune(words[i])))
	}
	names := nameColumn(labels) + 0.02
	tail := math.Min(0.3, float64(longest)*runeWidth/columnWidth+0.03)
	span := 1 - names - tail
	x := func(v float64) float64 { return names + norm(v, b.XAxis.Min, b.XAxis.Max)*span }
	slot := 1.0 / float64(len(b.Dumbbells))
	c.Height = rowsHeight(len(b.Dumbbells), 6)
	for _, t := range b.XAxis.Ticks {
		at := names + t.At*span
		c.Marks = append(c.Marks, document.Mark{Kind: document.LineMark, Tone: "line", Stroke: 0.4,
			Points: [][2]float64{{at, 0}, {at, 1}}})
		c.XTicks = append(c.XTicks, document.Tick{At: at, Label: t.Label})
	}
	// Every link before any dot, so each dot sits over the line it ends.
	for i, d := range b.Dumbbells {
		mid := (float64(i) + 0.5) * slot
		c.Marks = append(c.Marks, document.Mark{Kind: document.LineMark, Tone: "neutral", Stroke: 0.9,
			Points: [][2]float64{{x(d.From), mid}, {x(d.To), mid}}})
	}
	for i, d := range b.Dumbbells {
		mid := (float64(i) + 0.5) * slot
		from, to := x(d.From), x(d.To)
		c.Marks = append(c.Marks,
			document.Mark{Kind: document.DotMark, X: from, Y: mid, W: 0.005, Tone: "neutral",
				Label: d.Label, Value: d.FromSaid},
			document.Mark{Kind: document.DotMark, X: to, Y: mid, W: 0.005, Tone: tone(0),
				Label: d.Label, Value: d.ToSaid},
			name(d.Label, mid),
			text(1-tail+0.01, mid, words[i], "ink", document.StartAnchor, false))
	}
	if len(b.Ends) == 2 {
		c.Keys = []document.Key{{Tone: "neutral", Label: b.Ends[0]}, {Tone: tone(0), Label: b.Ends[1]}}
	}
}
