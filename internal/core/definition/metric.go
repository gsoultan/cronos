package definition

import "fmt"

// Metric is one number a chart draws, and how it draws it.
//
// A list of these is what a combo chart is: bars for billed, a line for the
// running average, against the same buckets. It is also the second shape a
// funnel comes in — stages that are separate columns rather than separate rows,
// which is how most warehouses model one.
//
// Distinct from MeasureRef, which is a pointer at a number and nothing else. A
// block's `y` is a MeasureRef because there is only one of it and nowhere for
// it to be drawn differently; these carry their own drawing.
// Named Metric rather than Measure because Role already owns that word: a
// field's role *is* `measure`, and a type sharing the name would shadow the
// constant every dataset is validated against.
type Metric struct {
	Field     string `json:"field" yaml:"field"`
	Aggregate string `json:"aggregate,omitempty" yaml:"aggregate,omitempty"`
	// Label is what a legend calls it. Empty falls back to the field's own
	// label, which is resolved where the datasets are.
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	// Draw is bar or line, on the chart types that mix them. Empty means bar.
	Draw string `json:"draw,omitempty" yaml:"draw,omitempty"`
	// Axis is "secondary" to read this measure against its own scale.
	//
	// Opt-in, per measure, and never a default. Two scales on one plot is the
	// most-flagged mistake in charting: where the two axes line up is a choice
	// nobody made on purpose, so the chart shows a correlation that is not in
	// the data. An author who needs one has to say so, which is the difference
	// between a considered decision and an accident.
	Axis string `json:"axis,omitempty" yaml:"axis,omitempty"`
}

// SecondaryAxis is the value of Axis that moves a measure to its own scale.
const SecondaryAxis = "secondary"

// Ref is the measure as a plain pointer, for the compiler.
func (m Metric) Ref() MeasureRef {
	return MeasureRef{Field: m.Field, Aggregate: m.Aggregate}
}

// Secondary reports whether this measure reads against its own scale.
func (m Metric) Secondary() bool { return m.Axis == SecondaryAxis }

// Line reports whether this measure is drawn as a line rather than bars.
func (m Metric) Line() bool { return m.Draw == "line" }

// Heading is what a legend calls it, or the field name when nothing else said.
func (m Metric) Heading() string {
	if m.Label != "" {
		return m.Label
	}
	return m.Field
}

// validate reports why the measure cannot be stored.
func (m Metric) validate(output string, i int, mixes bool) error {
	switch {
	case m.Field == "":
		return fmt.Errorf("%w: %s chart %d has a measure with no field", ErrInvalid, output, i)
	case m.Draw != "" && m.Draw != "bar" && m.Draw != "line":
		return fmt.Errorf("%w: %s chart %d draws a measure as %q, want bar or line",
			ErrInvalid, output, i, m.Draw)
	case m.Draw != "" && !mixes:
		return fmt.Errorf("%w: %s chart %d says how to draw a measure, which only a combo "+
			"chart chooses", ErrInvalid, output, i)
	case m.Axis != "" && m.Axis != SecondaryAxis:
		return fmt.Errorf("%w: %s chart %d puts a measure on axis %q, want secondary",
			ErrInvalid, output, i, m.Axis)
	case m.Secondary() && !mixes:
		return fmt.Errorf("%w: %s chart %d puts a measure on a second axis, which only a "+
			"combo chart has", ErrInvalid, output, i)
	}
	return nil
}
