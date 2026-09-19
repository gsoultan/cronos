package run

import (
	"fmt"
	"math"

	"github.com/gsoultan/cronos/internal/core/document"
)

// The arithmetic printable() leans on, kept apart from the arrangement
// decisions so each file reads as one thing.

// sliceSegments is how many vertices an arc is approximated with per turn.
//
// Typst has no arc primitive, so a pie slice is a polygon. Ninety-six around a
// full circle is a facet every 3.75°, which at the size a chart prints is under
// a tenth of a millimetre of deviation — invisible, and a fraction of the
// vertices a smooth-looking curve would suggest.
const sliceSegments = 96

// slice is one wedge of a circle in the unit box, from `from` turns for `span`
// turns. inner > 0 cuts the middle out, making it a ring segment.
func slice(from, span, inner float64) [][2]float64 {
	steps := maxInt(int(math.Abs(span)*sliceSegments), 2)
	outer := 0.5
	cx, cy := 0.5, 0.5

	at := func(turn, r float64) [2]float64 {
		a := turn * 2 * math.Pi
		return [2]float64{cx + math.Cos(a)*r, cy + math.Sin(a)*r}
	}

	pts := make([][2]float64, 0, steps*2+2)
	for i := 0; i <= steps; i++ {
		pts = append(pts, at(from+span*float64(i)/float64(steps), outer))
	}
	if inner <= 0 {
		// A solid wedge closes through the middle.
		return append(pts, [2]float64{cx, cy})
	}
	for i := steps; i >= 0; i-- {
		pts = append(pts, at(from+span*float64(i)/float64(steps), outer*inner))
	}
	return pts
}

// norm places v on lo..hi as a fraction, clamped.
func norm(v, lo, hi float64) float64 {
	if hi <= lo {
		return 0
	}
	return math.Max(0, math.Min(1, (v-lo)/(hi-lo)))
}

// share is v against the largest value, for the charts drawn without a scale.
func share(v, max float64) float64 {
	if max <= 0 {
		return 0
	}
	// A floor, so a value that is not zero is not drawn as nothing — a bar
	// that vanishes reads as a missing row rather than a small one.
	return math.Max(v/max, 0.004)
}

// at is where bucket i sits across the box. A lone bucket goes in the middle
// rather than on the left edge, where it reads as a series that lost its rest.
func at(i, count int) float64 {
	if count <= 1 {
		return 0.5
	}
	return float64(i) / float64(count-1)
}

// scaleOf is the vertical range to draw against: the server's axis where there
// is one, and the data's own span where there is not.
func scaleOf(a *Axis, groups []Group) (lo, hi float64) {
	if a != nil {
		return a.Min, a.Max
	}
	for _, g := range groups {
		for _, b := range g.Bars {
			lo, hi = math.Min(lo, b.Value), math.Max(hi, b.Value)
		}
	}
	if hi <= lo {
		hi = lo + 1
	}
	return lo, hi
}

// ticksOf carries the server's own tick labels onto the page, so the scale on
// a PDF reads the same as the scale in a browser — including its rounding.
func ticksOf(a *Axis) []document.Tick {
	if a == nil {
		return nil
	}
	out := make([]document.Tick, 0, len(a.Ticks))
	for _, t := range a.Ticks {
		out = append(out, document.Tick{At: t.At, Label: t.Label})
	}
	return out
}

func keysOf(groups []Group) []document.Key {
	if len(groups) < 2 {
		return nil
	}
	out := make([]document.Key, 0, len(groups))
	for _, g := range groups {
		out = append(out, document.Key{Tone: tone(g.Slot), Label: g.Label})
	}
	return out
}

func trackKeys(tracks []Track) []document.Key {
	out := make([]document.Key, 0, len(tracks))
	for _, t := range tracks {
		label := t.Label
		if t.Secondary {
			// Said on the page for the reason the viewer says it: a reader
			// otherwise has no way to know this line is not comparable to the
			// bars beside it.
			label += " (right)"
		}
		out = append(out, document.Key{Tone: tone(t.Slot), Label: label})
	}
	return out
}

func sliceKeys(series []Bar) []document.Key {
	out := make([]document.Key, 0, len(series))
	for i, s := range series {
		if s.Value > 0 {
			out = append(out, document.Key{Tone: tone(i), Label: s.Label + " " + s.Formatted})
		}
	}
	return out
}

// tone names a categorical colour, folding anything past the palette into the
// last slot rather than inventing a hue — the same rule the viewer follows.
func tone(slot int) string {
	return fmt.Sprintf("series-%d", minInt(maxInt(slot, 0), CategorySlots-1)+1)
}

// step names an ordinal colour, for the charts whose order carries meaning.
func step(rank int) string {
	return fmt.Sprintf("step-%d", minInt(maxInt(rank, 0), 4)+1)
}

func indexOf(list []string, v string) int {
	for i, s := range list {
		if s == v {
			return i
		}
	}
	return -1
}

func minOf(a, b float64) float64 { return math.Min(a, b) }
func maxOf(a, b float64) float64 { return math.Max(a, b) }
func sqrt(v float64) float64     { return math.Sqrt(v) }
func minInt(a, b int) int        { return min(a, b) }
func maxInt(a, b int) int        { return max(a, b) }
