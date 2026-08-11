package definition

// Block is one thing on a report.
//
// Dataset is per block. That is what replaced a separate Dashboard kind: a
// report whose blocks read different datasets *is* a dashboard, and giving it
// its own artifact kind only meant two things to learn — see
// docs/report-format.md.
//
// The fields are a union across the four kinds. A tagged union would be more
// precise and would make the YAML worse: authors write one flat mapping, and
// validateShape checks that a block only used the fields its kind reads.
type Block struct {
	Kind BlockKind `json:"kind" yaml:"kind"`
	// Dataset overrides the report's default. Empty means the report's.
	Dataset string `json:"dataset,omitempty" yaml:"dataset,omitempty"`

	// Text.
	Style string `json:"style,omitempty" yaml:"style,omitempty"`
	Text  string `json:"text,omitempty" yaml:"text,omitempty"`

	// Stat.
	Label string     `json:"label,omitempty" yaml:"label,omitempty"`
	Value MeasureRef `json:"value,omitzero" yaml:"value,omitempty"`
	// Filter narrows this block alone — "of which, overdue".
	Filter string `json:"filter,omitempty" yaml:"filter,omitempty"`

	// Chart.
	Title  string       `json:"title,omitempty" yaml:"title,omitempty"`
	Chart  string       `json:"chart,omitempty" yaml:"chart,omitempty"`
	X      DimensionRef `json:"x,omitzero" yaml:"x,omitempty"`
	Y      MeasureRef   `json:"y,omitzero" yaml:"y,omitempty"`
	Series DimensionRef `json:"series,omitzero" yaml:"series,omitempty"`

	// Table.
	Columns   []string  `json:"columns,omitempty" yaml:"columns,omitempty"`
	Sort      []SortKey `json:"sort,omitempty" yaml:"sort,omitempty"`
	PageSize  int       `json:"pageSize,omitempty" yaml:"pageSize,omitempty"`
	GroupBy   string    `json:"groupBy,omitempty" yaml:"groupBy,omitempty"`
	PageBreak string    `json:"pageBreak,omitempty" yaml:"pageBreak,omitempty"`
	Subtotals []string  `json:"subtotals,omitempty" yaml:"subtotals,omitempty"`
}

// DefaultPageSize is what a table returns when the author did not say.
//
// A number rather than everything: a block with no limit is how a report that
// worked on ten thousand rows falls over on ten million, and the author who
// wrote it will not be the one holding it.
const DefaultPageSize = 100

// Rows is the page size to actually use.
func (b Block) Rows() int {
	if b.PageSize > 0 {
		return b.PageSize
	}
	return DefaultPageSize
}

// DatasetFor resolves the block's dataset against the report's default.
func (b Block) DatasetFor(reportDefault string) string {
	if b.Dataset != "" {
		return b.Dataset
	}
	return reportDefault
}

// Heading is what a viewer labels the block with, whichever field carried it.
func (b Block) Heading() string {
	if b.Title != "" {
		return b.Title
	}
	return b.Label
}
