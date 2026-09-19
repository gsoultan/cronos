package run

// Group is one series of a chart that draws several.
//
// The bars of every group cover the same buckets in the same order, padded
// with zeroes where a series had no row. A viewer stacking two series cannot
// align them otherwise, and the alternative — each group carrying only the
// buckets it matched — pushes that alignment into every renderer.
type Group struct {
	Label string `json:"label"`
	// Slot is the categorical colour slot, from 0. Assigned by order of first
	// appearance and never by rank, so a filter that drops one series does not
	// repaint the ones that survive.
	Slot int   `json:"slot"`
	Bars []Bar `json:"bars"`
}
