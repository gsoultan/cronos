package definition

// Param is one question a dataset accepts.
//
// Params are the *only* caller-supplied input that reaches a query, and they
// reach it as bind arguments. There is deliberately no way to declare a
// parameter that substitutes SQL text: a report that needs a different shape
// of query is a different dataset, not a cleverer parameter.
type Param struct {
	Name     string    `json:"name" yaml:"name"`
	Type     ParamType `json:"type" yaml:"type"`
	Label    string    `json:"label,omitempty" yaml:"label,omitempty"`
	Required bool      `json:"required,omitempty" yaml:"required,omitempty"`
	// Multiple accepts a list, bound as a list — for `= ANY(...)` and `IN`.
	Multiple bool `json:"multiple,omitempty" yaml:"multiple,omitempty"`
	// Values enumerates the permitted values. Enum only, and required there:
	// an enum with no values accepts everything, which is the opposite of what
	// the author asked for.
	Values  []string `json:"values,omitempty" yaml:"values,omitempty"`
	Default any      `json:"default,omitempty" yaml:"default,omitempty"`
}

// HasDefault reports whether the param may be omitted by a caller.
func (p Param) HasDefault() bool { return p.Default != nil }
