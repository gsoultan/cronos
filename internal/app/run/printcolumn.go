package run

import (
	"github.com/gsoultan/cronos/internal/core/document"
)

// columns draws a column chart as the screen does: each bucket's columns
// standing on nothing, side by side per series or one stack of them, the
// figure over each where it has room, the categories under the box and the
// scale down its left. Stacked to a whole, each part is its share of its
// bucket and the scale is of percentages.
func columns(c *document.Chart, b Block) {
	groups := b.Groups
	if len(groups) == 0 {
		groups = []Group{{Label: b.Title, Bars: b.Series}}
	}
	buckets := len(groups[0].Bars)
	if buckets == 0 || b.YAxis == nil {
		return
	}
	lo, hi := b.YAxis.Min, b.YAxis.Max
	y := func(v float64) float64 { return 1 - norm(v, lo, hi) }
	for i := range buckets {
		if b.Stacked {
			columnStack(c, b, groups, i, y)
			continue
		}
		columnLanes(c, groups, i, y(minOf(maxOf(0, lo), hi)), y)
	}
	c.Ticks, c.XTicks = ticksOf(b.YAxis), categories(labelsOf(groups[0].Bars))
	c.Keys = keysOf(b.Groups)
}

// columnLanes stands each series' column for bucket i in a lane of its own.
func columnLanes(c *document.Chart, groups []Group, i int, zero float64, y func(float64) float64) {
	n := len(groups)
	slot := 1.0 / float64(len(groups[0].Bars))
	// Capped, as on screen: three months across a page were columns a third
	// of it wide, which reads as blocks of colour rather than heights.
	lane := minOf(slot*0.72/float64(n), 0.08)
	for k, g := range groups {
		v := g.Bars[i]
		// A series with nothing in this bucket contributes no mark, as in a
		// grouped bar chart: a sliver reads as a small amount, not as none.
		if n > 1 && v.Value == 0 {
			continue
		}
		left := float64(i)*slot + (slot-lane*float64(n))/2 + float64(k)*lane
		top := y(v.Value)
		c.Marks = append(c.Marks, document.Mark{Kind: document.RectMark,
			X: left + lane*0.06, Y: minOf(top, zero), W: lane * 0.88, H: maxOf(abs(zero-top), 0.004), Tone: tone(g.Slot)})
		if fits(v.Formatted, lane) {
			at := minOf(top, zero) - 0.05
			if v.Value < 0 {
				at = maxOf(top, zero) + 0.06
			}
			c.Marks = append(c.Marks, text(left+lane/2, at, v.Formatted, "ink", "", true))
		}
	}
}

// columnStack lays bucket i's parts end to end either side of nothing — as
// shares of the bucket when stacked to a whole — with its total over the top.
func columnStack(c *document.Chart, b Block, groups []Group, i int, y func(float64) float64) {
	w := minOf(0.64/float64(len(groups[0].Bars)), 0.09)
	left := middle(i, len(groups[0].Bars)) - w/2
	up, down := 0.0, 0.0
	for _, g := range groups {
		s := partOf(b, i, g.Bars[i].Value)
		if s == 0 {
			continue
		}
		from := up
		if s < 0 {
			from = down + s
			down += s
		} else {
			up += s
		}
		top := y(from + abs(s))
		c.Marks = append(c.Marks, document.Mark{Kind: document.RectMark,
			X: left, Y: top, W: w, H: maxOf(y(from)-top, 0.004), Tone: tone(g.Slot)})
	}
	if i < len(b.Totals) && fits(b.Totals[i].Formatted, w+0.02) {
		at := y(up) - 0.05
		if b.Totals[i].Value < 0 && !b.Percent {
			at = y(down) + 0.06
		}
		c.Marks = append(c.Marks, text(left+w/2, at, b.Totals[i].Formatted, "ink", "", true))
	}
}
