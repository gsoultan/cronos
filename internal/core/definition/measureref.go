package definition

// MeasureRef points a block at a number and says how to fold it.
//
// The aggregate is here rather than only on the field because one dataset
// measure legitimately appears twice on a report — a sum and a count of the
// same column — and the field's own aggregate is the default, not a law.
type MeasureRef struct {
	Field     string `json:"field" yaml:"field"`
	Aggregate string `json:"aggregate,omitempty" yaml:"aggregate,omitempty"`
}
