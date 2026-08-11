package definition

// Output is one profile of a report: what it renders to, and how it is laid
// out for that medium.
//
// A report has several because the same numbers become different documents. A
// statement on screen scrolls and filters; the same statement as a PDF breaks
// per customer and subtotals each one. Sharing a layout between them would
// mean neither is right — see docs/report-format.md.
type Output struct {
	Name     string       `json:"name" yaml:"name"`
	Renderer RendererKind `json:"renderer" yaml:"renderer"`
	Layout   []Block      `json:"layout,omitempty" yaml:"layout,omitempty"`

	// Paginated only.
	Page   PageSpec  `json:"page,omitzero" yaml:"page,omitempty"`
	Header Furniture `json:"header,omitzero" yaml:"header,omitempty"`
	Footer Furniture `json:"footer,omitzero" yaml:"footer,omitempty"`

	// Spreadsheet only.
	Sheets []Sheet `json:"sheets,omitempty" yaml:"sheets,omitempty"`
}
