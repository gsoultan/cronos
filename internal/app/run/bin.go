package run

// Bin is one bin of a histogram: where it starts and stops along the number,
// named as a range, and how many rows fell in it — or the measure folded over
// them — formatted by the engine that knew the unit.
type Bin struct {
	From      float64 `json:"from"`
	To        float64 `json:"to"`
	Label     string  `json:"label"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
}
