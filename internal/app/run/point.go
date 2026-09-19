package run

// Point is one dot of a scatter or bubble chart.
//
// X and Y stay numbers because the chart has to place them; the formatted
// pair is what a tooltip says. Both travel, for the same reason a Bar carries
// a value and a label.
type Point struct {
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	FX    string  `json:"fx"`
	FY    string  `json:"fy"`
	// Weight is the size measure scaled 0..1 across the chart, which is what a
	// bubble's radius reads. Zero on a scatter, which draws every dot alike.
	Weight float64 `json:"weight,omitempty"`
	// Size is the size measure formatted, when one is set.
	Size string `json:"size,omitempty"`
	Slot int    `json:"slot,omitempty"`
}
