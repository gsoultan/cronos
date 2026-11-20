package run

import "github.com/gsoultan/cronos/internal/core/document"

// printTimeline draws a timeline as the screen does: a row per bar, its name
// on the left, the bar between its dates along the calendar under the box,
// a faint rule up from each month so a bar's ends can be read against one,
// and a key where the bars are coloured by series.
func printTimeline(c *document.Chart, b Block) {
	if len(b.Spans) == 0 || b.XAxis == nil {
		return
	}
	labels := make([]string, len(b.Spans))
	for i, s := range b.Spans {
		labels[i] = s.Label
	}
	names := nameColumn(labels) + 0.02
	span := 1 - names
	x := func(v float64) float64 { return names + norm(v, b.XAxis.Min, b.XAxis.Max)*span }
	slot := 1.0 / float64(len(b.Spans))
	c.Height = rowsHeight(len(b.Spans), 6)
	for _, t := range b.XAxis.Ticks {
		at := names + t.At*span
		c.Marks = append(c.Marks, document.Mark{Kind: document.LineMark, Tone: "line", Stroke: 0.4,
			Points: [][2]float64{{at, 0}, {at, 1}}})
		c.XTicks = append(c.XTicks, document.Tick{At: at, Label: t.Label})
	}
	for i, s := range b.Spans {
		top, h := float64(i)*slot+slot*0.2, slot*0.6
		from, to := x(s.From), x(s.To)
		c.Marks = append(c.Marks,
			document.Mark{Kind: document.RectMark, X: from, Y: top, W: maxOf(to-from, 0.003), H: h,
				Tone: tone(s.Slot), Label: s.Label, Value: s.Said},
			name(s.Label, top+h/2))
	}
	c.Keys = spanKeys(b.Spans)
}

// spanKeys is a key per series, in the order they were first coloured.
func spanKeys(spans []Span) []document.Key {
	var out []document.Key
	seen := map[string]bool{}
	for _, s := range spans {
		if s.Series == "" || seen[s.Series] {
			continue
		}
		seen[s.Series] = true
		out = append(out, document.Key{Tone: tone(s.Slot), Label: s.Series})
	}
	if len(out) < 2 {
		return nil
	}
	return out
}
