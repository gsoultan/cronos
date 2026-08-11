package definition

// Field is one column of a dataset's result, as authors and reports see it.
//
// The name is the contract: reports bind to it, so renaming a field breaks
// every report that used it. Label is what people read and may change freely.
type Field struct {
	Name  string `json:"name" yaml:"name"`
	Type  string `json:"type" yaml:"type"`
	Role  Role   `json:"role" yaml:"role"`
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	// Hidden keeps a field out of the builder while leaving it available to
	// row scope predicates and joins — an id nobody should chart but every
	// predicate needs.
	Hidden    bool   `json:"hidden,omitempty" yaml:"hidden,omitempty"`
	Aggregate string `json:"aggregate,omitempty" yaml:"aggregate,omitempty"`
	Format    string `json:"format,omitempty" yaml:"format,omitempty"`
	// CurrencyField names the field carrying this measure's currency code, so
	// a total is never formatted in a currency it was not billed in.
	CurrencyField string `json:"currencyField,omitempty" yaml:"currencyField,omitempty"`
}
