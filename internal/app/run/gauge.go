package run

// Gauge is one number read against a target.
//
// Not a stat tile with extra fields: a stat says what a number is, and this
// says whether it is enough. The difference is the target, and every value
// here exists to answer that second question.
type Gauge struct {
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	Target    float64 `json:"target"`
	// TargetFormatted is the target as text, by the engine that knew the
	// currency — the same rule as every other displayable value.
	TargetFormatted string `json:"targetFormatted"`
	TargetLabel     string `json:"targetLabel"`
	// Share of the target, 0..1 for the arc. Capped, because an arc cannot
	// draw 180% and a gauge that wrapped past its own end would read as 80%.
	Share float64 `json:"share"`
	// Over is how far past the target, formatted, or empty when it is not.
	// Capping Share loses that, and "we beat it" is the news.
	Over string `json:"over,omitempty"`
}
