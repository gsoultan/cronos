package run

// Block is one rendered thing on a report.
//
// One struct across the kinds rather than a union, because this is JSON: a
// discriminated union would arrive in a browser as the same flat object with a
// kind field, and pretending otherwise in Go only adds a layer that has to be
// flattened again on the way out.
type Block struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`

	// Stat.
	Value string `json:"value,omitempty"`

	// Chart.
	Series []Bar `json:"series,omitempty"`

	// Table.
	Columns []Column   `json:"columns,omitempty"`
	Rows    [][]string `json:"rows,omitempty"`
	// Total is how many rows matched, which may exceed those returned. Saying
	// so beats letting someone conclude the report is wrong because they
	// counted fifty of twelve hundred.
	Total int `json:"total,omitempty"`

	Coverage *Coverage `json:"coverage,omitempty"`
}
