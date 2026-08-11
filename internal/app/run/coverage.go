package run

// Coverage is which shared filters reached a block, as a viewer needs it.
//
// Mirrors query.Coverage rather than re-exporting it: this is the wire
// contract, and a JSON shape that changes because a core type was refactored
// is a broken embed in someone else's application.
type Coverage struct {
	Applied []string `json:"applied,omitempty"`
	Ignored []string `json:"ignored,omitempty"`
}
