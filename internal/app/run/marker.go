package run

// Marker is one point on a map — a dot, a bubble, or a contribution to a
// density field, depending on which layers asked for it.
type Marker struct {
	Label     string  `json:"label"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	// Weight is the value scaled to 0..1 across the map, which is what a
	// bubble's radius and a heat layer's intensity both read. Computed here
	// because it needs every row, and a viewer streaming them would not have
	// the maximum until it had drawn most of them.
	Weight float64 `json:"weight"`
	// Size is the bubble measure, already formatted, when one is set.
	Size string `json:"size,omitempty"`
}

// A marker carries no colour slot. A map does not split by series — every
// layer already spends colour on magnitude, and a second encoding competing
// for the same channel is what makes a choropleth with coloured dots on it
// unreadable. ChartType.MultiSeries is where that is enforced.
