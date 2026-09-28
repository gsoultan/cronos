package definition

import "fmt"

// validateMarks checks what a map adds to its marks: a radius around its
// places, their names, and flows that move.
func (m MapSpec) validateMarks(output string, i int) error {
	radius := m.Draws(RadiusLayer)
	switch {
	case radius && (m.RadiusKm <= 0 || m.RadiusKm > MaxRadiusKm):
		return fmt.Errorf("%w: %s map %d draws a radius layer, which needs radiusKm "+
			"between 0 and %d", ErrInvalid, output, i, MaxRadiusKm)
	case m.RadiusKm != 0 && !radius:
		return fmt.Errorf("%w: %s map %d sets radiusKm but draws no radius layer",
			ErrInvalid, output, i)
	case m.Animate && !m.Draws(FlowLayer):
		return fmt.Errorf("%w: %s map %d animates its flows, and draws none", ErrInvalid, output, i)
	case m.Labels && !m.named():
		return fmt.Errorf("%w: %s map %d names its places, and draws no region or place "+
			"with a name — polygon, scatter, bubble, cluster or radius", ErrInvalid, output, i)
	}
	return nil
}

// named reports whether any layer draws a mark a label can name: a region,
// or a place. A hexagon, a route and a heat field have no one name each.
func (m MapSpec) named() bool {
	for _, l := range m.Resolved() {
		switch l {
		case PolygonLayer, ScatterLayer, BubbleLayer, ClusterLayer, RadiusLayer:
			return true
		}
	}
	return false
}
