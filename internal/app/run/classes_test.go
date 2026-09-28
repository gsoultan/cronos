package run

import (
	"math"
	"math/rand"
	"slices"
	"sort"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
How a map's values become its shades. Each method is checked for the property
that makes it worth choosing, and the diverging ramp for the one it exists
for: a fall and a rise are never the same colour.
*/

func spec(c definition.Classify, r definition.Ramp, breaks ...float64) *definition.MapSpec {
	return &definition.MapSpec{Classify: c, Ramp: r, Breaks: breaks}
}

// ckmeans is optimal: no way of cutting the values into runs costs less.
func TestNaturalBreaksAreTheBestSplitThereIs(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for trial := 0; trial < 300; trial++ {
		n := 2 + r.Intn(11)
		v := make([]float64, n)
		for i := range v {
			v[i] = math.Round(r.ExpFloat64()*100) / 10
		}
		sort.Float64s(v)
		k := 1 + r.Intn(min(6, n))
		got := ckmeans(v, k)
		if c, want := splitCost(v, got), cheapest(v, min(k, distinct(v))); c > want+1e-9 {
			t.Fatalf("%v into %d: costs %v, the best split costs %v (%v)", v, k, c, want, got)
		}
	}
}

// Values in three clumps break between the clumps, at round numbers.
func TestNaturalBreaksFallInTheGaps(t *testing.T) {
	values := []float64{1, 2, 3, 48, 50, 52, 97, 100, 101}
	got := classed(spec(definition.Jenks, ""), values, 3, math.NaN()).breaks
	if len(got) != 2 || got[0] < 3 || got[0] >= 48 || got[1] < 52 || got[1] >= 97 {
		t.Fatalf("breaks %v, want one between 3 and 48 and one between 52 and 97", got)
	}
	if got[0] != math.Trunc(got[0]) || got[1] != math.Trunc(got[1]) {
		t.Errorf("breaks %v between whole counts are not whole", got)
	}
}

func TestEqualIntervalsAreEqual(t *testing.T) {
	values := []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 100}
	got := classed(spec(definition.EqualInterval, ""), values, RampSteps, math.NaN()).breaks
	if want := []float64{17, 33, 50, 67, 83}; !slices.Equal(got, want) {
		t.Errorf("breaks %v, want %v", got, want)
	}
}

// A custom class keeps its colour whatever the data is: a threshold is the
// point, and a band that moved with the data would be a quantile again.
func TestACustomClassKeepsItsShade(t *testing.T) {
	m := spec(definition.CustomBreaks, "", 10, 20, 30, 40, 50)
	low := shadesFor(m, []float64{25, 26, 27})
	high := shadesFor(m, []float64{25, 45, 90})
	if low.shade(25) != 2 || high.shade(25) != 2 || high.shade(90) != 5 {
		t.Errorf("25 is shade %d in one report and %d in another; 90 is %d",
			low.shade(25), high.shade(25), high.shade(90))
	}
	// Bands wholly below the values are not keyed; the empty one between two
	// that are stays, so the scale reads without a gap.
	legend := high.legend([]float64{25, 45, 90})
	if len(legend) != 4 || legend[0].Step != 2 || legend[0].From != "20" || legend[3].To != "90" {
		t.Errorf("legend %+v", legend)
	}
}

// Diverging: below the midpoint cool, from it up warm, and the class nearest
// it the palest on each side — however lopsided the values are.
func TestADivergingRampKeepsFallsAndRisesApart(t *testing.T) {
	values := []float64{-30, -20, -10, -1, 1, 2, 5, 400}
	s := shadesFor(spec("", definition.DivergingRamp), values)
	for _, v := range values {
		if got := s.shade(v); (v < 0) != (got < RampSteps/2) {
			t.Errorf("%v is shade %d, on the wrong side of the midpoint", v, got)
		}
	}
	if s.shade(-1) != RampSteps/2-1 || s.shade(1) != RampSteps/2 {
		t.Errorf("the values nearest the midpoint are shades %d and %d, want the palest %d and %d",
			s.shade(-1), s.shade(1), RampSteps/2-1, RampSteps/2)
	}
	if s.shade(0) != RampSteps/2 {
		t.Errorf("a value on the midpoint is shade %d, want the palest above it", s.shade(0))
	}
	legend := s.legend(values)
	if legend[0].From != "-30" || legend[len(legend)-1].To != "400" {
		t.Errorf("legend %+v runs from the lowest value to the highest", legend)
	}
	for _, l := range legend {
		if l.Step < RampSteps/2 && figure(l.To) > 0 {
			t.Errorf("a cool band reaches above the midpoint: %+v", l)
		}
	}
}

// Every value above the midpoint: only warm shades, and a legend of them.
func TestADivergingRampWithOneSideHasOneSide(t *testing.T) {
	values := []float64{3, 5, 8, 13, 21}
	s := shadesFor(spec("", definition.DivergingRamp), values)
	for _, l := range s.legend(values) {
		if l.Step < RampSteps/2 {
			t.Errorf("a cool band for values that are all above the midpoint: %+v", l)
		}
	}
}

// A diverging ramp with custom breaks takes each side's from its own side.
func TestCustomBreaksOnADivergingRampSplitAtTheMidpoint(t *testing.T) {
	mid := 100.0
	m := spec(definition.CustomBreaks, definition.DivergingRamp, 50, 80, 100, 150)
	m.Midpoint = &mid
	s := shadesFor(m, []float64{10, 60, 90, 100, 120, 200})
	for v, want := range map[float64]int{10: 0, 60: 1, 90: 2, 100: 3, 120: 3, 200: 4} {
		if got := s.shade(v); got != want {
			t.Errorf("%v is shade %d, want %d", v, got, want)
		}
	}
}

func splitCost(v []float64, starts []int) float64 {
	total := 0.0
	for c, st := range starts {
		end := len(v)
		if c+1 < len(starts) {
			end = starts[c+1]
		}
		mean := 0.0
		for _, x := range v[st:end] {
			mean += x
		}
		mean /= float64(end - st)
		for _, x := range v[st:end] {
			total += (x - mean) * (x - mean)
		}
	}
	return total
}

// cheapest is the least cost of any cut of v into k runs, by trying them all.
func cheapest(v []float64, k int) float64 {
	best := math.Inf(1)
	var cut func(start, left int, starts []int)
	cut = func(start, left int, starts []int) {
		if left == 1 {
			best = math.Min(best, splitCost(v, append(starts, start)))
			return
		}
		for next := start + 1; next <= len(v)-left+1; next++ {
			cut(next, left-1, append(slices.Clone(starts), start))
		}
	}
	cut(0, k, nil)
	return best
}
