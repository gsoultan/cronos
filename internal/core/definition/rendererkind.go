package definition

// RendererKind selects the output class an output profile produces.
//
// Three classes, not three formats: `interactive` is anything a browser draws,
// `paginated` is anything with pages, `spreadsheet` is anything with cells.
// A new export format usually joins one of them rather than becoming a fourth.
type RendererKind string

const (
	Interactive RendererKind = "interactive"
	Paginated   RendererKind = "paginated"
	Spreadsheet RendererKind = "spreadsheet"
)

// Valid reports whether r is a renderer this build has.
func (r RendererKind) Valid() bool {
	switch r {
	case Interactive, Paginated, Spreadsheet:
		return true
	}
	return false
}
