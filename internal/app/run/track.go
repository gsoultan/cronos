package run

// Track is one measure of a combo chart, and how it is drawn.
//
// Dense, like a Group: every track covers every bucket in the same order, so a
// viewer reading bucket three off the bars and off the line is reading the
// same bucket. The alternative pushes that alignment into every renderer.
type Track struct {
	Label string `json:"label"`
	Slot  int    `json:"slot"`
	// Draw is "bar" or "line".
	Draw string `json:"draw"`
	// Secondary says this track reads against its own scale.
	//
	// Travels per track rather than as a flag on the block, because it is a
	// property of the measure: an author opts one measure out of the shared
	// scale, not the chart into having two.
	Secondary bool  `json:"secondary,omitempty"`
	Bars      []Bar `json:"bars"`
}
