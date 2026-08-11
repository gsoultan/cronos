package paginated

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed template.typ
var template []byte

const (
	mainFile = "main.typ"
	dataFile = "data.json"
	outFile  = "out.pdf"
)

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
func (r *Renderer) Render(ctx context.Context, doc Document, out io.Writer) error {
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

// stage writes the two files a render consists of. They are the only things in
// the root, so they are the only things the typesetter can read.
func (r *Renderer) stage(root string, doc Document) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("paginated: encode document: %w", err)
	}
	files := map[string][]byte{mainFile: template, dataFile: data}
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
