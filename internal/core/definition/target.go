package definition

import "fmt"

// Target is what a gauge measures its value against.
//
// Either a field or a constant, because both are ordinary. "Against this
// month's quota" is a column; "against 95%" is a number somebody agreed once
// and does not want to add a table for.
type Target struct {
	Field     string `json:"field,omitempty" yaml:"field,omitempty"`
	Aggregate string `json:"aggregate,omitempty" yaml:"aggregate,omitempty"`
	// Value is a fixed target. A pointer so that zero — a real target, and the
	// one a burn-down measures against — is distinguishable from unset.
	Value *float64 `json:"value,omitempty" yaml:"value,omitempty"`
	// Label names it on the gauge. Empty says "target".
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
}

// Fixed reports whether the target is a constant rather than a column.
func (t Target) Fixed() bool { return t.Value != nil }

// Set reports whether a target was given at all.
func (t Target) Set() bool { return t.Field != "" || t.Value != nil }

// Ref is the target as a measure, for the compiler. Only meaningful when it is
// a field.
func (t Target) Ref() MeasureRef {
	return MeasureRef{Field: t.Field, Aggregate: t.Aggregate}
}

// Heading is what the gauge calls it.
func (t Target) Heading() string {
	if t.Label != "" {
		return t.Label
	}
	return "Target"
}

// validate reports why the target cannot be stored.
func (t Target) validate(output string, i int) error {
	switch {
	case !t.Set():
		return fmt.Errorf("%w: %s gauge %d has no target — a gauge is a number against "+
			"something, and without one it is a stat tile", ErrInvalid, output, i)
	case t.Field != "" && t.Value != nil:
		// Silently preferring one would make the other look honoured.
		return fmt.Errorf("%w: %s gauge %d sets both a target field and a target value",
			ErrInvalid, output, i)
	case t.Fixed() && *t.Value == 0:
		return fmt.Errorf("%w: %s gauge %d has a target of zero, which nothing can be a "+
			"proportion of", ErrInvalid, output, i)
	}
	return nil
}
