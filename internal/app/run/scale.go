package run

import (
	"math"
	"sort"
)

// RampSteps is how many shades a choropleth uses.
//
// Six, not a continuous gradient. A reader answers "which of these regions is
// in the same band" far better than "is this blue slightly darker than that
// one", and six is about where a legend stops being readable at the size a
// report gives it.
const RampSteps = 6

// CategorySlots is how many series get their own colour.
//
// Past this the palette would have to invent hues, and invented hues are the
// ones that collide under colour-vision deficiency. A viewer folds the
// remainder into the last slot rather than cycling, because a cycled palette
// gives two different series the same colour with nothing saying they differ.
const CategorySlots = 8

// PlotSlots is the cap where every series is visible at once.
//
// A stacked bar puts its series in a fixed order, so only neighbours have to
// be told apart and the full eight hold. A scatter puts them all in the same
// space, where every pair is adjacent — and the palette's own validation only
// clears its first three slots against every pair. Past three, colour stops
// carrying the distinction.
const PlotSlots = 3

// steps buckets values into RampSteps by quantile.
//
// Quantile and not equal interval, because the measures a choropleth shades
// are almost always skewed: revenue by region has one capital city and forty
// others, and equal intervals paint thirty-nine of them the lightest shade and
// call it a map. Quantiles spend the ramp where the data is.
func steps(values []float64) []float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	breaks := make([]float64, 0, RampSteps-1)
	for i := 1; i < RampSteps; i++ {
		at := float64(i) / RampSteps * float64(len(sorted)-1)
		lo := int(at)
		frac := at - float64(lo)
		v := sorted[lo]
		if lo+1 < len(sorted) {
			v += (sorted[lo+1] - sorted[lo]) * frac
		}
		// Ties collapse: forty regions all at zero cannot be split into six
		// bands, and a legend claiming they can reads "0–0, 0–0, 0–0".
		if len(breaks) > 0 && v <= breaks[len(breaks)-1] {
			continue
		}
		breaks = append(breaks, v)
	}
	return breaks
}

// stepOf is which band v falls in, given the breaks.
func stepOf(v float64, breaks []float64) int {
	for i, b := range breaks {
		if v <= b {
			return i
		}
	}
	return len(breaks)
}

// weight scales v to 0..1 across lo..hi.
//
// Anchored at zero when the data is all positive, because a bubble's area
// reads as a quantity: scaling 90–100 across the full radius draws the 90 as a
// tenth of the 100, which is a nine-fold lie about a 10% difference.
func weight(v, lo, hi float64) float64 {
	if lo > 0 {
		lo = 0
	}
	if hi <= lo {
		return 0
	}
	return math.Max(0, math.Min(1, (v-lo)/(hi-lo)))
}

// span is the smallest and largest of values, or 0,0 for none.
func span(values []float64) (lo, hi float64) {
	if len(values) == 0 {
		return 0, 0
	}
	lo, hi = values[0], values[0]
	for _, v := range values[1:] {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return lo, hi
}

// axis builds the scale for a plot's axis: nice round ends, and ticks on them.
func axis(values []float64) Axis {
	lo, hi := span(values)
	if lo > 0 {
		lo = 0
	}
	if hi <= lo {
		// Every point at one value. A scale with no width places them all at
		// the same edge, which reads as a bug rather than as agreement.
		hi = lo + 1
	}
	step := niceStep((hi - lo) / 4)
	lo, hi = math.Floor(lo/step)*step, math.Ceil(hi/step)*step

	out := Axis{Min: lo, Max: hi}
	for v := lo; v <= hi+step/2; v += step {
		out.Ticks = append(out.Ticks, Tick{At: (v - lo) / (hi - lo), Label: compact(v)})
	}
	return out
}

// niceStep rounds a raw interval to 1, 2, 5 or 10 times a power of ten.
//
// Ticks at 0, 2.5, 5, 7.5 are arithmetically correct and nobody reads them.
// The powers people count in are the ones this snaps to.
func niceStep(raw float64) float64 {
	if raw <= 0 {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	switch n := raw / mag; {
	case n <= 1:
		return mag
	case n <= 2:
		return 2 * mag
	case n <= 5:
		return 5 * mag
	}
	return 10 * mag
}

// slots assigns a colour slot per distinct label, by order of first
// appearance and capped at cap.
//
// Order of appearance and not rank: colour follows the entity. A filter that
// removes the largest series must not repaint every series below it, because a
// reader comparing two screenshots would read the recolouring as a change in
// the data.
func slots(labels []string, cap int) map[string]int {
	out := make(map[string]int, len(labels))
	for _, l := range labels {
		if _, seen := out[l]; seen {
			continue
		}
		out[l] = min(len(out), cap-1)
	}
	return out
}
