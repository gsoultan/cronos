package run

// Filter describes one control on the filter bar.
type Filter struct {
	Name   string   `json:"name"`
	Label  string   `json:"label"`
	Type   string   `json:"type"`
	Values []string `json:"values,omitempty"`
}
