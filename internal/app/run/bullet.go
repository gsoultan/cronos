package run

// Bullet is one row of a bullet chart: a value and the target it is read
// against, for a category of x or for the whole set.
//
// A gauge in a row's height, without the gauge's cap: a bar can run past its
// target mark, where an arc would wrap past its own start, so the share is
// the reader's to see rather than a field to carry.
type Bullet struct {
	// Label is the category, and empty when the chart has no x — the panel's
	// title already says what its one bullet measures.
	Label     string  `json:"label"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	Target    float64 `json:"target"`
	// TargetFormatted is the target as text, by the engine that knew the
	// unit — the same rule as every other displayable value.
	TargetFormatted string `json:"targetFormatted"`
}
