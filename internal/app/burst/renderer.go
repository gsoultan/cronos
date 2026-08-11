package burst

import (
	"context"
	"io"

	"github.com/gsoultan/cronos/internal/core/document"
)

// Renderer typesets one recipient's document.
//
// Declared here because this is where it is consumed, and it takes a
// core document rather than anything the typesetter defines — a burst should
// read the same whether the output is a PDF, a spreadsheet or something that
// does not exist yet.
type Renderer interface {
	Render(ctx context.Context, doc document.Document, out io.Writer) error
}
