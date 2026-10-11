package definition

// BlockKind is what a block is.
//
// The chart *type* is a separate field: `kind: chart, chart: bar`. Splitting
// them means adding a chart type does not add a block kind, so every renderer
// keeps handling four cases rather than growing one per visualisation.
//
// A pivot is the one kind no renderer draws. It is a table whose columns come
// from the data, and it is crossed where the rows are read: what reaches a
// viewer, a PDF or a workbook is a table like any other.
type BlockKind string

const (
	StatBlock  BlockKind = "stat"
	ChartBlock BlockKind = "chart"
	TableBlock BlockKind = "table"
	TextBlock  BlockKind = "text"
	PivotBlock BlockKind = "pivot"
)

// Valid reports whether k is a kind a report may lay out.
func (k BlockKind) Valid() bool {
	switch k {
	case StatBlock, ChartBlock, TableBlock, TextBlock, PivotBlock:
		return true
	}
	return false
}
