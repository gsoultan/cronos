package document

// Column is one column of the group tables.
//
// Field is what the subtotal map is keyed by, so a column can be told whether
// it carries a total without the template knowing what a measure is.
type Column struct {
	Field string `json:"field"`
	Label string `json:"label"`
	// Align is "right" for anything numeric, "left" otherwise. Numbers that do
	// not share a right edge cannot be compared down a column.
	Align string `json:"align"`
}
