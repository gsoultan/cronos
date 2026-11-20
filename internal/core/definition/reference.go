package definition

import "fmt"

// Reference is a line across a chart's scale at a value somebody reads the
// marks against: a target, a threshold, the average of what is drawn.
//
// The commonest thing a business chart carries that a chart of the data alone
// does not — "are we above plan", "which months were below the SLA" — and
// otherwise drawn as a second series of one repeated number, which reads as
// data and is in the legend as if it were.
type Reference struct {
	// Value is a fixed figure, in the measure's own units: 500000 for a
	// monthly target, 0.85 for a share.
	Value *float64 `json:"value,omitempty" yaml:"value,omitempty"`
	// Stat is a figure of the marks themselves: avg, median, min or max of
	// what the chart draws, worked out after filters, so it moves with them.
	Stat string `json:"stat,omitempty" yaml:"stat,omitempty"`
	// Label is what the line is called beside its figure: "Target",
	// "SLA". A stat's line is named for the stat where this is empty.
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
}

// MaxReferences is the most lines a chart carries. Past three they stop
// being what the marks are read against and become a second chart.
const MaxReferences = 3

// referenceStats are the figures a reference can take from the marks.
var referenceStats = map[string]bool{"avg": true, "median": true, "min": true, "max": true}

// Referenced reports whether the chart type draws its marks against a value
// scale a line can cross: the ones with a vertical scale of the measure.
func (c ChartType) Referenced() bool {
	switch c {
	case LineChart, AreaChart, ColumnChart, ComboChart, WaterfallChart,
		BoxplotChart, ScatterChart, BubbleChart:
		return true
	}
	return false
}

func (b Block) validateReferences(output string, i int) error {
	if len(b.References) == 0 {
		return nil
	}
	switch {
	case !b.Chart.Referenced():
		return fmt.Errorf("%w: %s chart %d draws reference lines, which cross a value scale "+
			"and a %s chart has none — use a line, area, column, combo, waterfall, box plot "+
			"or scatter", ErrInvalid, output, i, b.Chart)
	case b.Stacked.Shares():
		return fmt.Errorf("%w: %s chart %d is stacked to its whole, whose scale is shares "+
			"rather than the measure a reference is in", ErrInvalid, output, i)
	case len(b.References) > MaxReferences:
		return fmt.Errorf("%w: %s chart %d draws %d reference lines, at most %d",
			ErrInvalid, output, i, len(b.References), MaxReferences)
	}
	for k, r := range b.References {
		if err := r.validate(output, i, k); err != nil {
			return err
		}
	}
	return nil
}

func (r Reference) validate(output string, i, k int) error {
	switch {
	case (r.Value == nil) == (r.Stat == ""):
		return fmt.Errorf("%w: %s chart %d reference %d needs a value or a stat, and not both",
			ErrInvalid, output, i, k)
	case r.Stat != "" && !referenceStats[r.Stat]:
		return fmt.Errorf("%w: %s chart %d reference %d takes stat %q, want avg, median, min or max",
			ErrInvalid, output, i, k, r.Stat)
	}
	return nil
}

// Heading is what the line is called on the chart.
func (r Reference) Heading() string {
	if r.Label != "" {
		return r.Label
	}
	switch r.Stat {
	case "avg":
		return "Average"
	case "median":
		return "Median"
	case "min":
		return "Lowest"
	case "max":
		return "Highest"
	}
	return ""
}
