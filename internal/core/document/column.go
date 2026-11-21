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
	// Bars is a data bar for each row of the group, -1 to 1 of the column's
	// largest, drawn behind the figure. Spark is a small line for each row,
	// its points 0 to 1 up the cell. Both are placed, never worked out, here.
	Bars  []float64   `json:"bars,omitempty"`
	Spark [][]float64 `json:"spark,omitempty"`
	// Shades is each row's ramp step from 0, or -1 for a cell left plain.
	Shades []int `json:"shades,omitempty"`
}
