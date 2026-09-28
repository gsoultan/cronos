package run

// SankeyNode is one category on one side of a sankey: where it sits down its
// side and how tall, as fractions of the box, and what flows through it.
type SankeyNode struct {
	Label string `json:"label"`
	// Side is 0 for a source, down the left, and 1 for a target, down the
	// right.
	Side      int     `json:"side"`
	Y         float64 `json:"y"`
	H         float64 `json:"h"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	// Slot is a source's categorical colour, which its bands carry across.
	Slot int `json:"slot"`
}
