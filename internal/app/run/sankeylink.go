package run

// SankeyLink is one band from a source to a target: where it leaves the
// source and where it meets the target, down each side, and how thick — all
// fractions of the box — and what it carries.
type SankeyLink struct {
	From      int     `json:"from"`
	To        int     `json:"to"`
	Y0        float64 `json:"y0"`
	Y1        float64 `json:"y1"`
	H         float64 `json:"h"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
}
