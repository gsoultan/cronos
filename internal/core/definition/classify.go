package definition

// Classify is how a shaded map layer's values — a region's, a route's, a
// hexagon's — are split into the ramp's shades.
type Classify string

const (
	// Quantile gives each shade the same number of places. The default,
	// because the measures a map shades are almost always skewed: revenue by
	// region has one capital and forty others, and equal widths paint the
	// forty in the lightest shade and call it a map.
	Quantile Classify = "quantile"
	// EqualInterval gives each shade the same width of values, for a measure
	// whose steps mean something in themselves — a percentage, a score.
	EqualInterval Classify = "equal"
	// Jenks breaks where the values themselves gap — natural breaks, found
	// optimally rather than approximated, so the same data always breaks the
	// same way.
	Jenks Classify = "jenks"
	// CustomBreaks are the author's own, from MapSpec.Breaks: a regulator's
	// thresholds, last year's bands.
	CustomBreaks Classify = "custom"
)

// RampShades is how many shades a map's ramp has. Six, not a gradient: a
// reader answers "are these in the same band" far better than "is this blue
// darker than that one", and six is where a legend stops being readable.
const RampShades = 6

// Valid reports whether c is a classification this build knows, or unset.
func (c Classify) Valid() bool {
	switch c {
	case "", Quantile, EqualInterval, Jenks, CustomBreaks:
		return true
	}
	return false
}
