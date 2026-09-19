package run

// Legend is one stop of a map's sequential ramp.
//
// The break is formatted here, by the engine that knew the currency — the same
// reason every other displayable value on the wire is a string. A legend
// reading "£0–£2.4k" and a tile reading "$2,400" for the same measure is the
// disagreement that makes a reader stop trusting both.
type Legend struct {
	// Step matches Shape.Step.
	Step int `json:"step"`
	// From is the lower bound of the bucket, formatted.
	From string `json:"from"`
	// To is the upper bound, formatted.
	To string `json:"to"`
}
