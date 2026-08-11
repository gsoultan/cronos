package run

// View is a rendered report: what a viewer draws, and nothing about how.
type View struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Filters     []Filter `json:"filters,omitempty"`
	Blocks      []Block  `json:"blocks"`
}
