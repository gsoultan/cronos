package run

// Arc is one flow, from somewhere to somewhere else.
type Arc struct {
	Label     string  `json:"label"`
	X1        float64 `json:"x1"`
	Y1        float64 `json:"y1"`
	X2        float64 `json:"x2"`
	Y2        float64 `json:"y2"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	// Weight scales the stroke, 0..1 across the map.
	Weight float64 `json:"weight"`
}
