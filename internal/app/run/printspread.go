package run

import (
	"github.com/gsoultan/cronos/internal/core/document"
)

// printBins draws a histogram as the screen does: a column per bin touching
// its neighbours along the number's own scale, each bin's count over it
// where it has room, and an empty bin left a gap in the range.
func printBins(c *document.Chart, b Block) {
	if len(b.Bins) == 0 || b.XAxis == nil || b.YAxis == nil {
		return
	}
	x := func(v float64) float64 { return norm(v, b.XAxis.Min, b.XAxis.Max) }
	y := func(v float64) float64 { return 1 - norm(v, b.YAxis.Min, b.YAxis.Max) }
	for _, bin := range b.Bins {
		if bin.Value == 0 {
			continue
		}
		left, right, top := x(bin.From), x(bin.To), y(bin.Value)
		c.Marks = append(c.Marks, document.Mark{Kind: document.RectMark,
			X: left + 0.001, Y: top, W: maxOf(right-left-0.002, 0.002), H: maxOf(y(0)-top, 0.004), Tone: "series-1"})
		if fits(bin.Formatted, right-left) {
			c.Marks = append(c.Marks, text((left+right)/2, top-0.05, bin.Formatted, "ink", "", true))
		}
	}
	c.Ticks, c.XTicks = ticksOf(b.YAxis), ticksOf(b.XAxis)
}

// printBoxes draws a box plot as the screen does: for each category the box
// from its lower quartile to its upper, washed and edged, the median firm
// across it, and whiskers with their caps — against the scale down the left.
func printBoxes(c *document.Chart, b Block) {
	if len(b.Boxes) == 0 || b.YAxis == nil {
		return
	}
	y := func(v float64) float64 { return 1 - norm(v, b.YAxis.Min, b.YAxis.Max) }
	n := len(b.Boxes)
	w := minOf(0.5/float64(n), 0.08)
	var labels []string
	for i, box := range b.Boxes {
		mid := middle(i, n)
		left, right := mid-w/2, mid+w/2
		top, bottom := y(box.Q3), y(box.Q1)
		c.Marks = append(c.Marks,
			stroke(0.6, "neutral", [2]float64{mid, y(box.High)}, [2]float64{mid, top}),
			stroke(0.6, "neutral", [2]float64{mid, bottom}, [2]float64{mid, y(box.Low)}),
			stroke(0.6, "neutral", [2]float64{mid - w/4, y(box.High)}, [2]float64{mid + w/4, y(box.High)}),
			stroke(0.6, "neutral", [2]float64{mid - w/4, y(box.Low)}, [2]float64{mid + w/4, y(box.Low)}),
			document.Mark{Kind: document.RectMark, X: left, Y: top, W: w, H: maxOf(bottom-top, 0.004), Tone: "series-1-wash"},
			stroke(0.8, "series-1", [2]float64{left, top}, [2]float64{right, top}, [2]float64{right, bottom},
				[2]float64{left, bottom}, [2]float64{left, top}),
			stroke(1.8, "series-1", [2]float64{left, y(box.Median)}, [2]float64{right, y(box.Median)}))
		if box.Label != "" {
			labels = append(labels, box.Label)
		}
	}
	c.Ticks = ticksOf(b.YAxis)
	if len(labels) == n {
		c.XTicks = categories(labels)
	}
}

// stroke is a line through points, at a width in points.
func stroke(width float64, tone string, points ...[2]float64) document.Mark {
	return document.Mark{Kind: document.LineMark, Tone: tone, Stroke: width, Points: points}
}
