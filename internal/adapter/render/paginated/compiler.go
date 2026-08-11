package paginated

import "context"

// Compiler typesets a prepared directory into a PDF.
//
// Declared here because this is where it is consumed: the renderer needs
// something that can turn source into a document, and does not care whether it
// is a subprocess, a pool of them, or an in-process library later.
//
// root is the only directory the implementation may let the typesetter read.
// That is the whole security boundary — see the package doc.
type Compiler interface {
	Compile(ctx context.Context, root, main, out string) error
}
