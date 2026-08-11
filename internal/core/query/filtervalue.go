package query

// FilterValue is what a caller sent for one shared filter.
//
// Separated from the filter's definition because they have different
// provenance and different trust: the definition is authored and validated on
// save, this arrives with the request.
type FilterValue struct {
	Op     Op    `json:"op"`
	Values []any `json:"values,omitempty"`
}
