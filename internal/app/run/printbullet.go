package run

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/document"
)

// bullets draws a bullet chart as the screen does: a row per category, its
// name on the left, a track shaded at the target's fractions, the value a
// bar along it and the target a mark across it, and the value, target and
// share on the right. Every row reads along the one scale under the box.
func bullets(c *document.Chart, b Block) {
	if len(b.Bullets) == 0 || b.XAxis == nil {
		return
	}
	labels := make([]string, len(b.Bullets))
	for i, r := range b.Bullets {
		labels[i] = r.Label
	}
	names := 0.0
	if len(nonEmpty(labels)) > 0 {
		names = nameColumn(labels) + 0.02
	}
	span := 1 - names - 0.26
	x := func(v float64) float64 { return names + norm(v, b.XAxis.Min, b.XAxis.Max)*span }
	slot := 1.0 / float64(len(b.Bullets))
	c.Height = rowsHeight(len(b.Bullets), 8)
	for i, r := range b.Bullets {
		top, h := float64(i)*slot+slot*0.18, slot*0.64
		bulletTrack(c, r, bandsOrDefault(b.Bands), b.XAxis, x, top, h)
		c.Marks = append(c.Marks,
			barMark(x, r.Value, top+h*0.32, h*0.36, "series-1"),
			document.Mark{Kind: document.LineMark, Tone: "ink", Stroke: 1.6,
				Points: [][2]float64{{x(r.Target), top + h*0.1}, {x(r.Target), top + h*0.9}}},
			name(r.Label, top+h/2),
			text(names+span+0.015, top+h/2, bulletWords(r), "ink", document.StartAnchor, false))
	}
	for _, t := range b.XAxis.Ticks {
		c.XTicks = append(c.XTicks, document.Tick{At: names + t.At*span, Label: t.Label})
	}
}

// bulletTrack shades a row's track at its target's fractions, darkest
// furthest from the target.
func bulletTrack(c *document.Chart, r Bullet, bands []float64, a *Axis, x func(float64) float64, top, h float64) {
	edges := []float64{a.Min}
	for _, f := range bands {
		edges = append(edges, f*r.Target)
	}
	edges = append(edges, a.Max)
	for k := 0; k+1 < len(edges); k++ {
		from, to := maxOf(a.Min, minOf(edges[k], a.Max)), maxOf(a.Min, minOf(edges[k+1], a.Max))
		if to <= from {
			continue
		}
		c.Marks = append(c.Marks, document.Mark{Kind: document.RectMark,
			X: x(from), Y: top, W: x(to) - x(from), H: h, Tone: fmt.Sprintf("shade-%d", minInt(k, 2)+1)})
	}
}

// bulletWords is what the right of a row says: the value, its target, and how
// far along it the value is.
func bulletWords(r Bullet) string {
	if r.Target == 0 {
		return r.Formatted
	}
	return fmt.Sprintf("%s of %s · %s", r.Formatted, r.TargetFormatted, percent(r.Value/r.Target))
}

func bandsOrDefault(bands []float64) []float64 {
	if len(bands) > 0 {
		return bands
	}
	return defaultBands
}

func nonEmpty(labels []string) []string {
	var out []string
	for _, l := range labels {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
