package definition

// BlockKind is what a block is.
//
// The chart *type* is a separate field: `kind: chart, chart: bar`. Splitting
// them means adding a chart type does not add a block kind, so every renderer
// keeps handling four cases rather than growing one per visualisation.
type BlockKind string

const (
	StatBlock  BlockKind = "stat"
	ChartBlock BlockKind = "chart"
	TableBlock BlockKind = "table"
	TextBlock  BlockKind = "text"
)

// Valid reports whether k is a kind every renderer implements.
func (k BlockKind) Valid() bool {
	switch k {
	case StatBlock, ChartBlock, TableBlock, TextBlock:
		return true
	}
	return false
}
