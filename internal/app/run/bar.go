package run

// Bar is one bar of a chart.
//
// Value stays a number because the chart compares them; Formatted carries the
// text beside it. The bar ranks, the label states.
type Bar struct {
	Label     string  `json:"label"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
}
