package run

import (
	"math"
	"sort"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
shades are how a map's shaded values become the ramp's colours, and the legend
that says which values each colour holds.

Sequential is one run of classes over every value. Diverging is two, either
side of the midpoint, each classed apart: below it the three cool shades, the
darkest furthest below; from it up the three warm ones. Classed together, a
map of changes with one large rise would put every fall and every small rise in
the same pale band — the distinction a diverging ramp exists to draw.
*/
type shades struct {
	low, high classes
	mid       float64
	diverging bool
}

// classes are one run of a ramp's shades: the breaks between them, a value on
// a break in the class below it, and the shade the first class is painted.
type classes struct {
	breaks []float64
	first  int
}

// shadesFor classes values as the map asks.
func shadesFor(m *definition.MapSpec, values []float64) shades {
	if m == nil || m.Ramp != definition.DivergingRamp {
		return shades{high: classed(m, values, RampSteps, math.NaN())}
	}
	mid := m.Middle()
	var below, above []float64
	for _, v := range values {
		if v < mid {
			below = append(below, v)
		} else {
			above = append(above, v)
		}
	}
	half := RampSteps / 2
	s := shades{mid: mid, diverging: true, high: classed(m, above, half, mid)}
	s.high.first = half
	s.low = classed(m, below, half, mid)
	// The class nearest the midpoint is the palest, however many there are.
	s.low.first = half - len(s.low.breaks) - 1
	return s
}

// shade is the colour v is painted.
func (s shades) shade(v float64) int {
	if s.diverging && v < s.mid {
		return s.low.first + stepOf(v, s.low.breaks)
	}
	return s.high.first + stepOf(v, s.high.breaks)
}

// legend names each shade with the values it covers, lowest first.
func (s shades) legend(values []float64) []Legend {
	if len(values) == 0 {
		return []Legend{}
	}
	lo, hi := span(values)
	if !s.diverging {
		return s.high.legend(lo, hi)
	}
	out := []Legend{}
	if lo < s.mid {
		out = append(out, s.low.legend(lo, s.mid)...)
	}
	if hi >= s.mid {
		out = append(out, s.high.legend(math.Max(lo, s.mid), hi)...)
	}
	return out
}

// legend labels each class of the run from its lower bound to its upper, the
// values' own ends at either end of it. A class wholly outside them is left
// out: a custom break below every value is a band of the author's scale, and a
// key to a shade no value here could be painted is a key to nothing. A class
// inside them is kept, empty or not, so the scale reads continuously.
func (c classes) legend(from, to float64) []Legend {
	out := make([]Legend, 0, len(c.breaks)+1)
	for j := 0; j <= len(c.breaks); j++ {
		lower, upper := from, to
		if j > 0 {
			lower = c.breaks[j-1]
		}
		if j < len(c.breaks) {
			upper = c.breaks[j]
		}
		if (j > 0 && lower >= to) || (j < len(c.breaks) && upper < from) {
			continue
		}
		out = append(out, Legend{Step: c.first + j, From: compact(lower), To: compact(upper)})
	}
	return out
}

// classed splits values into at most k classes by the map's method. mid is a
// diverging ramp's midpoint — the end of an equal interval's range on the
// side it bounds, and where custom breaks divide — or NaN for a sequential one.
func classed(m *definition.MapSpec, values []float64, k int, mid float64) classes {
	if len(values) == 0 {
		return classes{}
	}
	method := definition.Quantile
	if m != nil && m.Classify != "" {
		method = m.Classify
	}
	switch method {
	case definition.EqualInterval:
		return classes{breaks: equalBreaks(values, k, mid)}
	case definition.Jenks:
		return classes{breaks: jenksBreaks(values, k)}
	case definition.CustomBreaks:
		return classes{breaks: customBreaks(m.Breaks, values, mid)}
	}
	return classes{breaks: quantiles(values, k)}
}

// equalBreaks split the values' range into k of the same width — from the
// midpoint, on a diverging ramp, rather than from the value nearest it.
func equalBreaks(values []float64, k int, mid float64) []float64 {
	lo, hi := span(values)
	bottom, top := lo, hi
	if !math.IsNaN(mid) {
		if hi < mid {
			hi = mid
		} else {
			lo = mid
		}
	}
	whole := allWhole(values)
	out := make([]float64, 0, k-1)
	for j := 1; j < k; j++ {
		v := round3(lo + (hi-lo)*float64(j)/float64(k))
		if whole {
			v = math.Round(v)
		}
		// A class with nothing in it is a legend entry for no place.
		if v < bottom || v >= top || (len(out) > 0 && v <= out[len(out)-1]) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// jenksBreaks put a break in each of the k-1 widest natural gaps, at a round
// number inside the gap where there is one.
func jenksBreaks(values []float64, k int) []float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	starts := ckmeans(sorted, k)
	whole := allWhole(sorted)
	out := make([]float64, 0, len(starts))
	for _, at := range starts[1:] {
		out = append(out, inGap(sorted[at-1], sorted[at], whole))
	}
	return out
}

// inGap is a number at or above below and short of above — the round one
// halfway where rounding keeps it inside, below itself where it cannot.
func inGap(below, above float64, whole bool) float64 {
	v := round3((below + above) / 2)
	if whole {
		v = math.Floor(v)
	}
	if v < below || v >= above {
		return below
	}
	return v
}

// customBreaks are the author's, all of them — a class keeps its colour
// whatever this run's values are, which is the point of fixed thresholds — or,
// on a diverging ramp, the ones on the side of the midpoint these values are.
func customBreaks(breaks, values []float64, mid float64) []float64 {
	if math.IsNaN(mid) {
		return breaks
	}
	_, hi := span(values)
	out := make([]float64, 0, len(breaks))
	for _, b := range breaks {
		if b != mid && (b < mid) == (hi < mid) {
			out = append(out, b)
		}
	}
	return out
}
