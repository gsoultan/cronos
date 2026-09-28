package definition

import (
	"errors"
	"strings"
	"testing"
)

// shadedDrops is the drops map shading hexagons, which classify and ramp colour.
func shadedDrops(edit func(*MapSpec)) Block {
	b := drops()
	b.Map.Layers = []MapLayer{HexbinLayer}
	edit(b.Map)
	return b
}

func TestAMapClassesItsShadesAsAsked(t *testing.T) {
	mid := 100.0
	for name, edit := range map[string]func(*MapSpec){
		"equal":         func(m *MapSpec) { m.Classify = EqualInterval },
		"natural":       func(m *MapSpec) { m.Classify = Jenks },
		"custom":        func(m *MapSpec) { m.Classify, m.Breaks = CustomBreaks, []float64{10, 100, 1000} },
		"diverging":     func(m *MapSpec) { m.Ramp = DivergingRamp },
		"about a point": func(m *MapSpec) { m.Ramp, m.Midpoint = DivergingRamp, &mid },
		"custom either side": func(m *MapSpec) {
			m.Ramp, m.Midpoint, m.Classify = DivergingRamp, &mid, CustomBreaks
			m.Breaks = []float64{50, 80, 100, 120, 150}
		},
	} {
		if err := mapReport(shadedDrops(edit)).Validate(); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
}

// Each is refused with a sentence rather than drawn as something nobody asked
// for — a classification quietly ignored is a map that looks deliberate.
func TestAMapIsRefusedShadesItCannotMean(t *testing.T) {
	mid := 100.0
	for name, c := range map[string]struct {
		edit func(*MapSpec)
		says string
	}{
		"a method nobody built": {func(m *MapSpec) { m.Classify = "kmeans" }, "want quantile"},
		"a ramp nobody built":   {func(m *MapSpec) { m.Ramp = "rainbow" }, "want sequential"},
		"breaks with no custom": {func(m *MapSpec) { m.Breaks = []float64{1} }, "only read with classify: custom"},
		"custom with no breaks": {func(m *MapSpec) { m.Classify = CustomBreaks }, "between 1 and 5"},
		"more breaks than shades": {func(m *MapSpec) {
			m.Classify, m.Breaks = CustomBreaks, []float64{1, 2, 3, 4, 5, 6}
		}, "between 1 and 5"},
		"breaks out of order": {func(m *MapSpec) {
			m.Classify, m.Breaks = CustomBreaks, []float64{10, 5}
		}, "larger than the one before"},
		"a midpoint on one hue": {func(m *MapSpec) { m.Midpoint = &mid }, "only a diverging ramp"},
		"diverging breaks that miss the midpoint": {func(m *MapSpec) {
			m.Ramp, m.Midpoint, m.Classify, m.Breaks = DivergingRamp, &mid, CustomBreaks, []float64{50, 150}
		}, "include its midpoint"},
		"three breaks below the midpoint": {func(m *MapSpec) {
			m.Ramp, m.Midpoint, m.Classify = DivergingRamp, &mid, CustomBreaks
			m.Breaks = []float64{10, 20, 30, 100}
		}, "at most 2 either side"},
	} {
		err := mapReport(shadedDrops(c.edit)).Validate()
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want it to mention %q", name, err, c.says)
		}
	}
	// Dots take no shade: a classification on them would do nothing.
	b := drops()
	b.Map.Classify = Jenks
	if err := mapReport(b).Validate(); err == nil || !strings.Contains(err.Error(), "no shaded layer") {
		t.Errorf("jenks on dots: %v", err)
	}
}
