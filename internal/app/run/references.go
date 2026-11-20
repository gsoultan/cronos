package run

import (
	"math"
	"slices"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// reference works out the block's reference lines and widens its value scale
// to take them: a target above every month is the point of drawing one, and
// a line clipped off the top of the plot says the opposite.
//
// The stats are of what is drawn — after filters, of the stacks where the
// chart stacks — so the average line moves with the marks it sits among.
func (b *Block) reference(blk definition.Block) {
	if len(blk.References) == 0 || b.YAxis == nil {
		return
	}
	drawn := b.plotted()
	var at []float64
	for _, r := range blk.References {
		v, ok := refValue(r, drawn)
		if !ok {
			continue
		}
		said := compact(v)
		if whole(drawn) {
			// The average of counts is written as a count: 267 tickets a
			// month, not 266.67, which claims a precision nobody counted.
			said = compact(math.Round(v))
		}
		b.References = append(b.References, RefLine{Label: r.Heading(), Value: v, Formatted: said})
		at = append(at, v)
	}
	if len(at) == 0 {
		return
	}
	lo, hi := slices.Min(at), slices.Max(at)
	if lo >= b.YAxis.Min && hi <= b.YAxis.Max {
		return
	}
	widened := niceAxis(math.Min(lo, b.YAxis.Min), math.Max(hi, b.YAxis.Max),
		append(at, b.YAxis.Min, b.YAxis.Max))
	b.YAxis = &widened
}

func refValue(r definition.Reference, drawn []float64) (float64, bool) {
	if r.Value != nil {
		return *r.Value, true
	}
	if len(drawn) == 0 {
		// Nothing drawn has no average. A line at zero would be a claim.
		return 0, false
	}
	switch r.Stat {
	case "min":
		return slices.Min(drawn), true
	case "max":
		return slices.Max(drawn), true
	case "median":
		s := slices.Sorted(slices.Values(drawn))
		n := len(s)
		if n%2 == 1 {
			return s[n/2], true
		}
		return (s[n/2-1] + s[n/2]) / 2, true
	}
	var sum float64
	for _, v := range drawn {
		sum += v
	}
	return sum / float64(len(drawn)), true
}

// plotted is every value the chart draws against its left scale.
func (b *Block) plotted() []float64 {
	var out []float64
	switch {
	case b.Tracks != nil:
		for _, t := range b.Tracks {
			if !t.Secondary {
				out = append(out, valuesOf(t.Bars)...)
			}
		}
	case b.Steps != nil:
		for _, s := range b.Steps {
			if !s.Total {
				out = append(out, s.Value)
			}
		}
	case b.Boxes != nil:
		for _, x := range b.Boxes {
			out = append(out, x.Median)
		}
	case b.Points != nil:
		for _, p := range b.Points {
			out = append(out, p.Y)
		}
	default:
		out = b.heights()
	}
	return out
}

func valuesOf(bars []Bar) []float64 {
	out := make([]float64, 0, len(bars))
	for _, b := range bars {
		out = append(out, b.Value)
	}
	return out
}
