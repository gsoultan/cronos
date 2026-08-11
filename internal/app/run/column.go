package run

// Column is a table heading, and how its cells line up.
type Column struct {
	Label string `json:"label"`
	// Align is "right" for measures. Numbers that do not share a right edge
	// cannot be compared down a column.
	Align string `json:"align,omitempty"`
}
