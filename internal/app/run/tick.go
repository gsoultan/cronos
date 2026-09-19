package run

// Tick is one labelled position on an axis.
type Tick struct {
	// At is where the tick sits, 0 at Min and 1 at Max.
	At float64 `json:"at"`
	// Label is already formatted.
	Label string `json:"label"`
}
