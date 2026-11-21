package definition

import "fmt"

// Faceted reports whether a chart of this type can be drawn as small
// multiples: the ones read along a shared scale, where a series per panel is
// still the same chart.
func (c ChartType) Faceted() bool {
	return c == LineChart || c == AreaChart || c == ColumnChart
}

// validateFacet reports why a chart cannot be split into small multiples.
//
// A facet is a panel per series, so it needs a series; and a stack is the
// series added up, which a panel each takes apart — the two ask for opposite
// drawings of the same parts.
func (b Block) validateFacet(output string, i int) error {
	switch {
	case !b.Facet:
		return nil
	case !b.Chart.Faceted():
		return fmt.Errorf("%w: %s chart %d is faceted, which a line, an area or a column "+
			"chart can be and a %s chart cannot", ErrInvalid, output, i, b.Chart)
	case b.Series.Field == "":
		return fmt.Errorf("%w: %s chart %d is faceted by nothing — set series, the field "+
			"each small chart is one value of", ErrInvalid, output, i)
	case b.Stacked.On():
		return fmt.Errorf("%w: %s chart %d is both stacked and faceted — a stack adds the "+
			"series up and a facet draws them apart", ErrInvalid, output, i)
	}
	return nil
}
