package document

// Furniture is a running header or footer a report asked for in place of the
// built-in one.
//
// Text is one line, with {{page}} and {{pages}} counted within the recipient's
// own statement. Template is the source of a Typst file defining header(ctx)
// or footer(ctx); it is staged beside the document rather than carried in it,
// so it is not on the wire.
type Furniture struct {
	Text     string `json:"text,omitempty"`
	Template string `json:"-"`
}

// Set reports whether the report asked for anything.
func (f Furniture) Set() bool { return f.Text != "" || f.Template != "" }
