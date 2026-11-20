package run

// Column is a table heading, and how its cells line up.
type Column struct {
	Label string `json:"label"`
	// Align is "right" for measures. Numbers that do not share a right edge
	// cannot be compared down a column.
	Align string `json:"align,omitempty"`
	// Bars is a data bar for each row, as the row's figure over the largest
	// figure in the column, so -1 to 1: drawn behind the cell, a figure's
	// length read against its neighbours' without reading any of them. A
	// cell that is not a number has none.
	//
	// The fraction rather than the figure, because the figure is already the
	// cell, written in its format, and the largest is a thing the server has
	// and each of three renderers would otherwise find again.
	Bars []float64 `json:"bars,omitempty"`
	// Spark is a small line for each row, its points across the column's
	// width at 0 to 1 up its height, each row against its own lowest and
	// highest: the shape of the row, not its size. Its cells are empty.
	Spark [][]float64 `json:"spark,omitempty"`
}
