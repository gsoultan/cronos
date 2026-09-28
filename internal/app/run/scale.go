package run

import (
	"math"
	"sort"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// RampSteps is how many shades a choropleth uses — see definition.RampShades,
// which the format validates custom breaks against.
const RampSteps = definition.RampShades

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
	return quantiles(values, RampSteps)
}

// quantiles are the breaks splitting values into k classes of as near the
// same count as ties allow.
func quantiles(values []float64, k int) []float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	top := sorted[len(sorted)-1]
	// Whole things are counted in whole numbers: a break between 12 trucks
	// and 14 read "13.30", which is a truck and a third.
	whole := allWhole(sorted)
	breaks := make([]float64, 0, k-1)
	for i := 1; i < k; i++ {
		at := float64(i) / float64(k) * float64(len(sorted)-1)
		lo := int(at)
		frac := at - float64(lo)
		v, next := sorted[lo], sorted[lo]
		if lo+1 < len(sorted) {
			next = sorted[lo+1]
			v += (next - sorted[lo]) * frac
		}
		// Rounded, and kept between the two values it was interpolated from.
		// Rounding alone could leave them: 1,004 rounds to 1,000, below every
		// value of a set that starts at 1,004 — one band for all of it, and a
		// legend reading "1,004–1,000".
		v = round3(v)
		if whole {
			v = math.Round(v)
		}
		v = min(max(v, sorted[lo]), next)
		// Ties collapse: forty regions all at zero cannot be split into six
		// bands, and a legend claiming they can reads "0–0, 0–0, 0–0". A break
		// at the top collapses too — the band above it would hold nothing.
		if v >= top || (len(breaks) > 0 && v <= breaks[len(breaks)-1]) {
			continue
		}
		breaks = append(breaks, v)
	}
	return breaks
}

// allWhole reports whether every value is a whole number.
func allWhole(values []float64) bool {
	for _, v := range values {
		if v != math.Trunc(v) {
			return false
		}
	}
	return true
}

// round3 is v to three significant figures.
//
// A break interpolated between two values is a number nobody chose, and its
// digits past the third are noise: the legend read "9,800–14,333.33". Rounded
// here, where the break is made, rather than where it is printed, so a region
// shaded in a band is always inside the range the legend gives that band.
//
// Divided rather than multiplied below one: 0.1 has no exact binary form, so
// 360 × 0.1 is 36.00000000000001 — which compact prints as "36.00".
func round3(v float64) float64 {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	exp := int(math.Floor(math.Log10(math.Abs(v)))) - 2
	if exp < -300 {
		// Past float64's exponent, the power of ten below is infinite and
		// the rounding a NaN. Nothing that small is on a map.
		return v
	}
	if exp >= 0 {
		p := math.Pow10(exp)
		return math.Round(v/p) * p
	}
	p := math.Pow10(-exp)
	return math.Round(v*p) / p
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
//
// To four decimals: the fifth moves the largest bubble's edge by a hundredth
// of a pixel, and the sixteen a float prints were a sixth of every place a
// map sends.
func weight(v, lo, hi float64) float64 {
	if lo > 0 {
		lo = 0
	}
	if hi <= lo {
		return 0
	}
	return math.Round(math.Max(0, math.Min(1, (v-lo)/(hi-lo)))*1e4) / 1e4
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
	if step < 1 && whole(values) {
		// A count has no half. Ticks at 0.5 on an axis of invoices say there
		// could be half of one, and a reader believes the axis.
		step = 1
	}
	lo, hi = math.Floor(lo/step)*step, math.Ceil(hi/step)*step

	out := Axis{Min: lo, Max: hi}
	for v := lo; v <= hi+step/2; v += step {
		out.Ticks = append(out.Ticks, Tick{At: (v - lo) / (hi - lo), Label: compact(v)})
	}
	return out
}

// whole reports whether every value is a whole number.
func whole(values []float64) bool {
	for _, v := range values {
		if v != math.Trunc(v) {
			return false
		}
	}
	return true
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
