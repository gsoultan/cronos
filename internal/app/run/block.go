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
	//
	// Empty, rather than absent, on the chart types that carry their points in
	// one of the fields below instead. The rule is the same one: `series` is
	// the chart kind's collection, so it is always there.
	Series []Bar `json:"series"`
	// Groups is one entry per series when the block splits by a dimension.
	Groups []Group `json:"groups,omitempty"`
	// Stacked says a multi-series bar or area is drawn as one stack per
	// bucket. A property of the drawing rather than of the data, which is why
	// it travels beside the groups instead of being inferred from them.
	Stacked bool `json:"stacked,omitempty"`
	// Totals is the height of each stack, one per bucket, formatted.
	//
	// Sent rather than added up in the viewer. A viewer summing the segments
	// has the numbers but not the currency or the rounding, so the figure it
	// writes beside a stack is the one number on the report this engine did
	// not format — and the first to disagree with the PDF.
	Totals []Bar `json:"totals,omitempty"`
	// Points are the dots of a scatter or a bubble, with the scales to place
	// them against.
	Points []Point `json:"points,omitempty"`
	XAxis  *Axis   `json:"xAxis,omitempty"`
	YAxis  *Axis   `json:"yAxis,omitempty"`
	// Map is every layer a map block draws.
	Map *GeoMap `json:"map,omitempty"`
	// Tracks is one entry per measure of a combo chart.
	Tracks []Track `json:"tracks,omitempty"`
	// Axis2 is the second scale, when a measure opted out of the shared one.
	Axis2 *Axis `json:"axis2,omitempty"`
	// Stages, Steps, Cells and Rects are the funnel, the waterfall, the
	// heatmap and the treemap. One field each rather than a shared list of
	// anonymous marks: what a stage carries and what a step carries have
	// almost nothing in common, and a union of them would be a struct whose
	// meaning depended on a field two levels up.
	Stages []Stage `json:"stages,omitempty"`
	Steps  []Step  `json:"steps,omitempty"`
	Cells  []Cell  `json:"cells,omitempty"`
	Rects  []Rect  `json:"rects,omitempty"`
	// Rows and Columns label a heatmap's grid, in draw order.
	HeatRows    []string `json:"heatRows,omitempty"`
	HeatColumns []string `json:"heatColumns,omitempty"`
	// Gauge is one number against a target.
	Gauge *Gauge `json:"gauge,omitempty"`

	// Table. Also never omitempty, for the same reason.
	Columns []Column   `json:"columns"`
	Rows    [][]string `json:"rows"`
	// Total is how many rows matched, which may exceed those returned. Saying
	// so beats letting someone conclude the report is wrong because they
	// counted fifty of twelve hundred.
	Total int `json:"total,omitempty"`

	Coverage *Coverage `json:"coverage,omitempty"`
}
