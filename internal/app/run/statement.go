package run

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/document"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Statements turns a report's paginated output into a rendered document.
//
// It sits between run and the typesetter because the mapping is a reporting
// decision rather than a typesetting one: which block becomes the table, which
// columns it carries, what the group is called. The renderer only draws.
type Statements struct {
	svc      *Service
	renderer burst.Renderer
}

// NewStatements wires the bridge.
func NewStatements(s *Service, r burst.Renderer) *Statements {
	return &Statements{svc: s, renderer: r}
}

// Statement renders one recipient's document.
//
// The recipient is already decided: params carry their id, and the dataset's
// row scope carries the rest. This produces a document with exactly one group,
// which is what makes "Page 1 of 2" mean the recipient's own statement rather
// than the run's — see internal/adapter/render/paginated.
func (s *Statements) Statement(ctx context.Context, r definition.Report, output string,
	params map[string]any, pr principal.Principal) (burst.StatementResult, error) {

	out, ok := r.Output(output)
	if !ok {
		return burst.StatementResult{}, fmt.Errorf("%w: report %q has no output %q",
			ErrNotRenderable, r.Name, output)
	}
	if out.Renderer != definition.Paginated {
		return burst.StatementResult{}, fmt.Errorf("%w: output %q renders %s, not a document",
			ErrNotRenderable, output, out.Renderer)
	}

	view, err := s.svc.Render(ctx, r, Request{Output: output, Params: params}, pr)
	if err != nil {
		return burst.StatementResult{}, err
	}

	doc, rows, err := toDocument(r, out, view)
	if err != nil {
		return burst.StatementResult{}, err
	}

	var buf bytes.Buffer
	if err := s.renderer.Render(ctx, doc, &buf); err != nil {
		return burst.StatementResult{}, err
	}
	return burst.StatementResult{Document: buf.Bytes(), Rows: rows}, nil
}

// toDocument maps a rendered view onto the paginated model.
func toDocument(r definition.Report, out definition.Output, view View) (document.Document, int, error) {
	table, ok := firstTable(view)
	if !ok {
		// A paginated output with no table is a page of headings. Refusing
		// beats delivering an empty PDF that looks like a customer who was
		// billed nothing.
		return document.Document{}, 0, fmt.Errorf("%w: output %q has no table to print",
			ErrNotRenderable, out.Name)
	}

	doc := document.Document{
		Title:   r.Heading(),
		Period:  view.Description,
		Org:     document.Org{Name: r.Folder},
		Page:    page(out.Page),
		Columns: columns(table),
		Groups: []document.Group{{
			Label:    view.Title,
			Rows:     table.Rows,
			Subtotal: subtotals(view, table),
		}},
	}
	return doc, len(table.Rows), doc.Validate()
}

func firstTable(view View) (Block, bool) {
	for _, b := range view.Blocks {
		if b.Kind == string(definition.TableBlock) {
			return b, true
		}
	}
	return Block{}, false
}

func columns(table Block) []document.Column {
	cols := make([]document.Column, 0, len(table.Columns))
	for _, c := range table.Columns {
		align := c.Align
		if align == "" {
			align = "left"
		}
		cols = append(cols, document.Column{Field: c.Label, Label: c.Label, Align: align})
	}
	return cols
}

// subtotals carries the report's own stat tiles onto the statement's total
// row, so the figure a customer reads at the bottom is the one the engine
// computed rather than a second sum of the rows that were printed.
func subtotals(view View, table Block) map[string]string {
	right := ""
	for _, c := range table.Columns {
		if c.Align == "right" {
			right = c.Label
		}
	}
	if right == "" {
		return nil
	}
	for _, b := range view.Blocks {
		if b.Kind == string(definition.StatBlock) && b.Value != "" {
			return map[string]string{right: b.Value}
		}
	}
	return nil
}

func page(spec definition.PageSpec) document.Page {
	return document.Page{
		Size:        paper(spec.Size),
		Orientation: spec.Orientation,
		MarginMM:    millimetres(spec.Margins),
	}
}
