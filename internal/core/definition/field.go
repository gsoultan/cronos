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
	// Format is how the field's figures are written wherever they appear —
	// a tile, a bar's label, an axis, a table cell, a PDF: currency, percent
	// (the value is a fraction, 0.873 written 87.3%), number, or preformatted
	// for a column the query already wrote as text. Empty is plain.
	Format string `json:"format,omitempty" yaml:"format,omitempty"`
	// Currency is the ISO 4217 code a currency field is written in, so a
	// total reads "€6.2M" rather than a bare number somebody has to ask about.
	Currency string `json:"currency,omitempty" yaml:"currency,omitempty"`
	// Decimals fixes how many places a figure keeps, where the format's own
	// choice is not the one wanted: a rating to one place, a rate to three.
	Decimals *int `json:"decimals,omitempty" yaml:"decimals,omitempty"`
	// CurrencyField names the field carrying this measure's currency code, so
	// a total is never formatted in a currency it was not billed in.
	CurrencyField string `json:"currencyField,omitempty" yaml:"currencyField,omitempty"`
}
