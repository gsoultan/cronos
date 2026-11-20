package run

// RefLine is a reference line as drawn: what it is called, where it crosses
// the scale, and its figure written as the measure's figures are.
type RefLine struct {
	Label     string  `json:"label"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
}
