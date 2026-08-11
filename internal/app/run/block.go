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

	// Chart. Kind stays "chart" and the type travels beside it, so a line
	// chart later is a new value here rather than a new kind every renderer
	// has to learn.
	Chart string `json:"chart,omitempty"`
	// Never omitempty. A nil slice would drop the key entirely, and a viewer
	// reading `series.map` on a report that matched no rows crashes on the
	// emptiest, most ordinary case there is. An empty array is data; a missing
	// field is a question.
	Series []Bar `json:"series"`

	// Table. Also never omitempty, for the same reason.
	Columns []Column   `json:"columns"`
	Rows    [][]string `json:"rows"`
	// Total is how many rows matched, which may exceed those returned. Saying
	// so beats letting someone conclude the report is wrong because they
	// counted fifty of twelve hundred.
	Total int `json:"total,omitempty"`

	Coverage *Coverage `json:"coverage,omitempty"`
}
