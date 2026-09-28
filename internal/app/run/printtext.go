package run

import (
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/gsoultan/cronos/internal/core/document"
)

// The words a printed chart carries, placed where the screen places them, and
// the scales a bar is drawn against once its name and its number have room.

const (
	// nameEnd is the widest a name column gets, as a share of the box.
	nameEnd = 0.26
	// valueRoom is what is kept past the longest bar for its number, and
	// before zero when a bar runs below it.
	valueRoom = 0.13
	// mostLabels is as many category labels as a printed chart's width holds.
	mostLabels = 12
	// nameRunes is as long as a name gets in its column before it is cut.
	nameRunes = 24
	// columnWidth is the narrowest a chart is printed, in millimetres: a
	// portrait A4's column. Whether words fit is judged against it, so what
	// fits there fits on a landscape page too.
	columnWidth = 170.0
	// runeWidth is about what a character of a chart's 6pt type takes, in
	// millimetres — a figure a little more than a letter.
	runeWidth = 1.25
	// pointRadius is a line's or a scatter's point, as a share of the width.
	pointRadius = 0.0042
)

// nameColumn is as wide as the longest name needs, within what a page can
// give up to names: a fixed quarter of the box left "England" floating a
// hand's width from its bar on a landscape page.
func nameColumn(labels []string) float64 {
	longest := 0
	for _, l := range labels {
		longest = max(longest, min(utf8.RuneCountInString(l), nameRunes))
	}
	return math.Min(nameEnd, math.Max(0.08, float64(longest)*runeWidth/columnWidth+0.02))
}

// fits reports whether a label has room across a share of the box.
func fits(label string, share float64) bool {
	return share*columnWidth >= float64(utf8.RuneCountInString(label))*runeWidth+2
}

// barScale places a value across the room a bar has: from the end of the name
// column, and from further in when some bars run below zero and need room for
// their numbers on the left, to where the numbers on the right begin.
func barScale(values []float64, names float64) func(float64) float64 {
	lo, hi := 0.0, 0.0
	for _, v := range values {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if hi <= lo {
		hi = lo + 1
	}
	x0 := names + 0.02
	if lo < 0 {
		x0 += valueRoom
	}
	span := 1 - valueRoom - x0
	return func(v float64) float64 { return x0 + (v-lo)/(hi-lo)*span }
}

// groupEnds is every value a grouped chart's bars reach — the ends of each
// stack, either side of nothing, when it is stacked.
func groupEnds(b Block) []float64 {
	var out []float64
	if len(b.Groups) == 0 {
		return out
	}
	for i := range b.Groups[0].Bars {
		up, down := 0.0, 0.0
		for _, g := range b.Groups {
			v := g.Bars[i].Value
			if !b.Stacked {
				out = append(out, v)
			} else if v < 0 {
				down += v
			} else {
				up += v
			}
		}
		if b.Stacked {
			out = append(out, up, down)
		}
	}
	return out
}

// barMark is one bar from nothing to v, a sliver at least: a bar that
// vanishes reads as a missing row rather than a small one.
func barMark(x func(float64) float64, v, y, h float64, tone string) document.Mark {
	from, to := x(0), x(v)
	return document.Mark{Kind: document.RectMark, X: math.Min(from, to), Y: y,
		W: math.Max(math.Abs(to-from), 0.003), H: h, Tone: tone}
}

// zeroLine marks nothing, where a bar runs below it.
func zeroLine(c *document.Chart, x func(float64) float64, values []float64) {
	for _, v := range values {
		if v < 0 {
			c.Marks = append(c.Marks, document.Mark{Kind: document.LineMark, Tone: "neutral", Stroke: 0.4,
				Points: [][2]float64{{x(0), 0}, {x(0), 1}}})
			return
		}
	}
}

// text is a label at x, y: its middle there unless anchored at an end.
func text(x, y float64, label, tone, anchor string, strong bool) document.Mark {
	return document.Mark{Kind: document.TextMark, X: x, Y: y, Label: label, Tone: tone,
		Anchor: anchor, Strong: strong}
}

// headline is the one figure a chart is about — a donut's whole, a gauge's
// reading — set large in the middle, and smaller where it would run past
// most of the width: a total of millions at the size of one of thousands ran
// out over the ring.
func headline(y float64, label string, size, most float64) document.Mark {
	m := text(0.5, y, label, "ink", "", true)
	m.Size, m.W = size, most
	return m
}

// inkOn is the ink a label is set in over a tone: near-black over the light
// shades and the palette's one light hue, white over the rest.
func inkOn(tone string) string {
	switch tone {
	case "ramp-1", "ramp-2", "ramp-3", "step-1", "series-4", "frame", "line":
		return "ink"
	}
	return "white"
}

// runesIn is how many characters of a chart's type a share of the box holds.
func runesIn(share float64) int {
	return max(int((share*columnWidth-2)/runeWidth), 0)
}

// name is a category's name in the column on the left, cut to fit it.
func name(label string, y float64) document.Mark {
	return text(0, y, cut(label, nameRunes), "ink", document.StartAnchor, false)
}

// valueAt is a bar's number just past its end: right of a gain, left of a
// loss.
func valueAt(x func(float64) float64, v float64, label string, y float64) document.Mark {
	if v < 0 {
		return text(x(v)-0.008, y, label, "ink", document.EndAnchor, true)
	}
	return text(x(v)+0.008, y, label, "ink", document.StartAnchor, true)
}

// middle is the middle of bucket i of count, where a column stands and a
// line's point sits.
func middle(i, count int) float64 {
	return (float64(i) + 0.5) / float64(maxInt(count, 1))
}

// categories labels each bucket under its middle, or every second or third
// where more than a printed width holds would run into each other.
func categories(labels []string) []document.Tick {
	every := maxInt(1, int(math.Ceil(float64(len(labels))/mostLabels)))
	out := make([]document.Tick, 0, len(labels)/every+1)
	for i, l := range labels {
		if i%every == 0 {
			out = append(out, document.Tick{At: middle(i, len(labels)), Label: cut(l, 14)})
		}
	}
	return out
}

// shifted moves ticks laid across a whole box into the part of it right of
// left, for a grid with its row names in the rest.
func shifted(ticks []document.Tick, left float64) []document.Tick {
	for i := range ticks {
		ticks[i].At = left + ticks[i].At*(1-left)
	}
	return ticks
}

func labelsOf(bars []Bar) []string {
	out := make([]string, len(bars))
	for i, b := range bars {
		out[i] = b.Label
	}
	return out
}

// cut shortens a label to n characters, marking where it was cut.
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// percent is a share as a reader reads one: to a tenth, and whole when it is.
func percent(share float64) string {
	p := math.Round(share*1000) / 10
	if p == math.Trunc(p) {
		return fmt.Sprintf("%.0f%%", p)
	}
	return fmt.Sprintf("%.1f%%", p)
}

func abs(v float64) float64 { return math.Abs(v) }

// rowsHeight is the box a chart of rows needs, in millimetres: each row its
// height, within what a page can take.
func rowsHeight(rows int, each float64) float64 {
	return math.Max(16, math.Min(float64(rows)*each+2, 150))
}
