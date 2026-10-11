package paginated

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gsoultan/cronos/internal/core/document"
)

//go:embed template.typ
var template []byte

const (
	mainFile   = "main.typ"
	dataFile   = "data.json"
	outFile    = "out.pdf"
	headerFile = "header.typ"
	footerFile = "footer.typ"
)

// staged is the document as the template reads it: the document, and whether
// a header or footer template was written beside it. The template cannot ask
// whether a file exists without failing when it does not, so it is told.
type staged struct {
	document.Document
	HeaderFile bool `json:"headerFile"`
	FooterFile bool `json:"footerFile"`
}

// Renderer turns a Document into a PDF.
//
// It holds no state between renders and is safe for concurrent use: each call
// gets its own directory, so two bursts cannot read each other's data even by
// accident.
type Renderer struct {
	compiler Compiler
}

// New returns a Renderer driven by c.
func New(c Compiler) *Renderer {
	return &Renderer{compiler: c}
}

// Render typesets doc and writes the PDF to out.
func (r *Renderer) Render(ctx context.Context, doc document.Document, out io.Writer) error {
	if err := doc.Validate(); err != nil {
		return err
	}

	root, err := os.MkdirTemp("", "cronos-render-*")
	if err != nil {
		return fmt.Errorf("paginated: render root: %w", err)
	}
	defer os.RemoveAll(root)

	if err := r.stage(root, doc); err != nil {
		return err
	}

	pdf := filepath.Join(root, outFile)
	if err := r.compiler.Compile(ctx, root, mainFile, pdf); err != nil {
		return err
	}
	return copyOut(pdf, out)
}

// stage writes the files a render consists of: the template, the document,
// and a report's own header and footer templates if it has them. They are the
// only things in the root, so they are the only things the typesetter can read
// — an author's template included.
func (r *Renderer) stage(root string, doc document.Document) error {
	data, err := json.Marshal(staged{Document: doc,
		HeaderFile: doc.Header.Template != "", FooterFile: doc.Footer.Template != ""})
	if err != nil {
		return fmt.Errorf("paginated: encode document: %w", err)
	}
	files := map[string][]byte{mainFile: template, dataFile: data}
	if doc.Header.Template != "" {
		files[headerFile] = []byte(doc.Header.Template)
	}
	if doc.Footer.Template != "" {
		files[footerFile] = []byte(doc.Footer.Template)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), content, 0o600); err != nil {
			return fmt.Errorf("paginated: write %s: %w", name, err)
		}
	}
	return nil
}

func copyOut(pdf string, out io.Writer) error {
	f, err := os.Open(pdf)
	if err != nil {
		return fmt.Errorf("paginated: typst produced no output: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(out, f); err != nil {
		return fmt.Errorf("paginated: write pdf: %w", err)
	}
	return nil
}
