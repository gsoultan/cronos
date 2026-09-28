package definition

// Ramp is what a shaded map's colours say: more, or which side of a middle.
type Ramp string

const (
	// SequentialRamp is one hue, darker for more. The default.
	SequentialRamp Ramp = "sequential"
	// DivergingRamp is two hues either side of a midpoint, for a measure
	// whose middle means something — a change on last year, a margin, a
	// score around zero. On one hue, a fall and a small rise are neighbours.
	DivergingRamp Ramp = "diverging"
)

// Valid reports whether r is a ramp this build knows, or unset.
func (r Ramp) Valid() bool {
	return r == "" || r == SequentialRamp || r == DivergingRamp
}
