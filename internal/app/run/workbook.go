package run

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// Workbooks writes a spreadsheet from a report's spreadsheet output.
//
// Declared as a port rather than importing the writer, so run stays a use case
// and the XLSX details stay in the adapter that knows them.
type Workbooks interface {
	Write(w io.Writer, sheets []SheetData) error
}

// SheetData is one tab, as the use case assembled it.
type SheetData struct {
	Name         string
	Headers      []string
	Rows         [][]Value
	FreezeHeader bool
	AutoFilter   bool
}

// Value is one cell before a writer decides how to store it.
type Value struct {
	Text    string
	Number  float64
	Numeric bool
}

// Spreadsheet renders a report's spreadsheet output.
func (s *Statements) Spreadsheet(ctx context.Context, r definition.Report, output string,
	params map[string]any, pr principal.Principal) ([]byte, error) {

	out, ok := r.Output(output)
	if !ok {
		return nil, fmt.Errorf("%w: report %q has no output %q", ErrNotRenderable, r.Name, output)
	}
	if out.Renderer != definition.Spreadsheet {
		return nil, fmt.Errorf("%w: output %q renders %s, not a spreadsheet",
			ErrNotRenderable, output, out.Renderer)
	}

	sheets := make([]SheetData, 0, len(out.Sheets))
	for _, spec := range out.Sheets {
		sheet, err := s.sheet(ctx, r, spec, params, pr)
		if err != nil {
			return nil, err
		}
		sheets = append(sheets, sheet)
	}

	var buf bytes.Buffer
	if err := s.workbooks.Write(&buf, sheets); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sheet runs one tab's query.
func (s *Statements) sheet(ctx context.Context, r definition.Report, spec definition.Sheet,
	params map[string]any, pr principal.Principal) (SheetData, error) {

	ds, err := s.svc.datasets.Dataset(ctx, r.Dataset)
	if err != nil {
		return SheetData{}, err
	}

	blk := definition.Block{
		Kind: definition.TableBlock, Columns: spec.Columns, PageSize: ExportLimit,
	}
	cols, rows, err := s.svc.BlockRows(ctx, ds, blk, params, query.Filters{Defs: r.Filters}, pr)
	if err != nil {
		return SheetData{}, err
	}

	data := SheetData{
		Name: spec.Name, Headers: headings(blk, ds, cols).labels(),
		FreezeHeader: spec.FreezeHeader, AutoFilter: spec.AutoFilter,
	}
	for _, row := range rows {
		cells := make([]Value, len(row))
		for i, v := range row {
			cells[i] = value(v)
		}
		data.Rows = append(data.Rows, cells)
	}
	return data, nil
}

// value decides whether a cell is a number.
//
// From what the driver returned rather than from the field's declared role: a
// measure that came back as a formatted string is a string, and writing it as
// a number would be a lie the spreadsheet then does arithmetic on.
func value(v any) Value {
	switch v.(type) {
	case float64, float32, int, int64, int32:
		n, _ := number(v)
		return Value{Number: n, Numeric: true, Text: cell(v)}
	}
	return Value{Text: cell(v)}
}
