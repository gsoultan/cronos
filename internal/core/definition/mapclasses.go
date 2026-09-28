package definition

import (
	"fmt"
	"math"
)

// validateClasses checks how a map's shaded values are classed and coloured.
func (m MapSpec) validateClasses(output string, i int) error {
	set := m.Classify != "" || len(m.Breaks) > 0 || m.Ramp != "" || m.Midpoint != nil
	switch {
	case !m.Classify.Valid():
		return fmt.Errorf("%w: %s map %d classify %q, want quantile, equal, jenks or custom",
			ErrInvalid, output, i, m.Classify)
	case !m.Ramp.Valid():
		return fmt.Errorf("%w: %s map %d ramp %q, want sequential or diverging",
			ErrInvalid, output, i, m.Ramp)
	case set && !m.shaded():
		return fmt.Errorf("%w: %s map %d sets how its values are shaded, and draws no "+
			"shaded layer — polygon, line or hexbin", ErrInvalid, output, i)
	case m.Midpoint != nil && m.Ramp != DivergingRamp:
		return fmt.Errorf("%w: %s map %d sets a midpoint, which only a diverging ramp has",
			ErrInvalid, output, i)
	case m.Midpoint != nil && (math.IsNaN(*m.Midpoint) || math.IsInf(*m.Midpoint, 0)):
		return fmt.Errorf("%w: %s map %d midpoint is not a number", ErrInvalid, output, i)
	case len(m.Breaks) > 0 && m.Classify != CustomBreaks:
		return fmt.Errorf("%w: %s map %d lists breaks, which are only read with "+
			"classify: custom", ErrInvalid, output, i)
	case m.Classify == CustomBreaks:
		return m.validateBreaks(output, i)
	}
	return nil
}

// validateBreaks checks custom breaks: ascending, and no more than the ramp
// has shades to put between — on a diverging ramp, no more than half on
// either side of its midpoint, which has to be one of them.
func (m MapSpec) validateBreaks(output string, i int) error {
	b := m.Breaks
	if len(b) == 0 || len(b) > RampShades-1 {
		return fmt.Errorf("%w: %s map %d classify: custom needs between 1 and %d breaks, "+
			"for up to %d shades", ErrInvalid, output, i, RampShades-1, RampShades)
	}
	for k, v := range b {
		if math.IsNaN(v) || math.IsInf(v, 0) || (k > 0 && v <= b[k-1]) {
			return fmt.Errorf("%w: %s map %d breaks must be numbers, each larger than the "+
				"one before", ErrInvalid, output, i)
		}
	}
	if m.Ramp != DivergingRamp {
		return nil
	}
	mid, below, above, on := m.Middle(), 0, 0, false
	for _, v := range b {
		switch {
		case v < mid:
			below++
		case v > mid:
			above++
		default:
			on = true
		}
	}
	if !on || below > RampShades/2-1 || above > RampShades/2-1 {
		return fmt.Errorf("%w: %s map %d is diverging, so its breaks include its midpoint "+
			"%g and at most %d either side of it", ErrInvalid, output, i, mid, RampShades/2-1)
	}
	return nil
}

// Middle is a diverging ramp's midpoint: the author's, or zero.
func (m MapSpec) Middle() float64 {
	if m.Midpoint != nil {
		return *m.Midpoint
	}
	return 0
}

// shaded reports whether any layer colours its marks from the ramp.
func (m MapSpec) shaded() bool {
	return m.geometric() || m.Draws(HexbinLayer) || m.Draws(H3Layer)
}
