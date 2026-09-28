package run

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// Rounding a break to three figures could move it out of the data: 1,004
// rounds to 1,000, below every value of a set that starts at 1,004. Every
// region then fell in one band, the map was one colour, and the legend read
// "1,004–1,000".
func TestABreakStaysInsideTheValuesItWasTakenFrom(t *testing.T) {
	for _, values := range [][]float64{
		{1004, 1004, 1004, 1004, 1004, 1004, 1100},
		{12345, 12346, 12347, 12348, 12349, 12350, 12351, 12352, 12353, 12354},
	} {
		lo, hi := span(values)
		breaks := steps(values)
		for _, b := range breaks {
			if b < lo || b >= hi {
				t.Errorf("%v: break %v is outside %v–%v", values, b, lo, hi)
			}
		}
		if stepOf(lo, breaks) == stepOf(hi, breaks) {
			t.Errorf("%v: the smallest and the largest share a band (breaks %v)", values, breaks)
		}
		for _, l := range shadesFor(nil, values).legend(values) {
			if figure(l.From) > figure(l.To) {
				t.Errorf("%v: a band reads backwards, %s–%s", values, l.From, l.To)
			}
		}
	}
}

func TestRoundingTheSmallestNumbersDoesNotMakeThemNaN(t *testing.T) {
	if v := round3(5e-310); v != 5e-310 {
		t.Errorf("round3(5e-310) = %v", v)
	}
}

// figure reads a formatted number back, for comparing two of them.
func figure(s string) float64 {
	v, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return v
}

// Whole things are counted in whole numbers. A break between 12 trucks and 14
// read "13.30", which is a truck and a third.
func TestABreakBetweenWholeCountsIsAWholeCount(t *testing.T) {
	values := []float64{9, 12, 14, 23, 29, 33, 38, 41, 47, 52, 64}
	for _, b := range steps(values) {
		if b != math.Trunc(b) {
			t.Errorf("break %v between whole counts", b)
		}
	}
	// Data with fractions in it keeps them: a break between 0.5 and 1.25 is
	// not rounded to 1 just because 1 is whole.
	fractional := false
	for _, b := range steps([]float64{0.5, 0.75, 1.25, 1.5, 2.75, 3.25, 3.5}) {
		fractional = fractional || b != math.Trunc(b)
	}
	if !fractional {
		t.Error("breaks between fractions were all rounded to whole numbers")
	}
}

// An axis of whole counts ticks at whole counts: one invoice a month drew a
// scale of 0, 0.5 and 1, as if there could be half an invoice.
func TestAnAxisOfCountsHasNoHalves(t *testing.T) {
	for _, values := range [][]float64{{1, 1, 1, 1}, {0, 1}, {0, 2, 3}} {
		for _, tick := range axis(values).Ticks {
			if strings.ContainsAny(tick.Label, ".") {
				t.Errorf("%v: a tick at %q", values, tick.Label)
			}
		}
	}
	// A measure with fractions still gets the steps it needs.
	if ticks := axis([]float64{0.2, 0.9}).Ticks; len(ticks) < 3 {
		t.Errorf("fractions got %d ticks", len(ticks))
	}
}
