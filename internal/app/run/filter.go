package run

// Filter describes one control on the filter bar.
type Filter struct {
	Name   string   `json:"name"`
	Label  string   `json:"label"`
	Type   string   `json:"type"`
	Values []string `json:"values,omitempty"`
	// Control is which interface to operate it through, already resolved to
	// the type's default where the author named none.
	//
	// Resolved here rather than on the wire's far side so that the portal, the
	// embed and anything else draw the same filter the same way — a default
	// applied independently in three places is three defaults.
	Control string `json:"control"`
}
