package run

// Stage is one step of a funnel.
//
// Share and Drop are computed here because both are ratios of other rows, and
// a viewer that had to derive them would need the whole funnel in hand before
// it could draw the first stage — and would round them differently from the
// PDF of the same report.
type Stage struct {
	Label     string  `json:"label"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	// Share of the first stage, 0..1. The width of the band.
	Share float64 `json:"share"`
	// Drop from the previous stage, already formatted as a percentage. Empty
	// on the first stage, which has nothing to have fallen from.
	Drop string `json:"drop,omitempty"`
}
