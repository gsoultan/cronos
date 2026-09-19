package run

// Step is one bar of a waterfall.
//
// Start and End are the running total either side of this step, so a viewer
// places the floating bar without accumulating anything itself — which is what
// stops the last bar of a long waterfall drifting by a rounding error the
// server never made.
type Step struct {
	Label     string  `json:"label"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	Start     float64 `json:"start"`
	End       float64 `json:"end"`
	// Sign is -1, 0 or 1. Sent rather than derived from Value because the
	// total row has a sign of zero while carrying a positive value, and a
	// viewer comparing against zero would paint it as a rise.
	Sign int `json:"sign"`
	// Total marks the closing bar, which is drawn from the baseline rather
	// than floating.
	Total bool `json:"total,omitempty"`
}
