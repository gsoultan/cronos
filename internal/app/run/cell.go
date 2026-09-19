package run

// Cell is one square of a heatmap.
//
// Carries its own row and column rather than an index into two lists: a client
// reading a sparse grid should not have to decide what a missing (row, column)
// pair means, and the reader fills every pair in so there are none.
type Cell struct {
	Row       string  `json:"row"`
	Column    string  `json:"column"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	// Step is which stop of the sequential ramp shades it, from 0.
	Step int `json:"step"`
	// Empty marks a pair no row matched, which is drawn as absence rather than
	// as the lightest shade — "none" and "nearly none" are different answers.
	Empty bool `json:"empty,omitempty"`
}
