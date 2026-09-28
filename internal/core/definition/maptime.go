package definition

import "fmt"

// timeGrains are the periods a timed map can play through — the grains a
// date axis buckets by.
var timeGrains = map[string]bool{"day": true, "week": true, "month": true, "quarter": true, "year": true}

// validateTime checks a map that plays through periods.
func (m MapSpec) validateTime(output string, i int) error {
	t := m.Time
	switch {
	case t == nil:
		return nil
	case t.Field == "":
		return fmt.Errorf("%w: %s map %d plays through time but names no field", ErrInvalid, output, i)
	case !timeGrains[t.Grain]:
		return fmt.Errorf("%w: %s map %d time grain %q, want day, week, month, quarter or year",
			ErrInvalid, output, i, t.Grain)
	case m.Draws(HexbinLayer) || m.Draws(H3Layer):
		return fmt.Errorf("%w: %s map %d plays through time, and hexagons and H3 cells are "+
			"folded from every place at once — give them a block of their own", ErrInvalid, output, i)
	}
	return nil
}
