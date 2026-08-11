package definition

// SortKey is one level of a table's ordering.
//
// A list, because "newest first, then largest" is one intent and expressing it
// as a single field loses the tiebreak — which is how two runs of the same
// report come back in different orders.
type SortKey struct {
	Field string `json:"field" yaml:"field"`
	// Dir is asc or desc. Empty means asc.
	Dir string `json:"dir,omitempty" yaml:"dir,omitempty"`
}
