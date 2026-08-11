package spreadsheet

import (
	"io"

	"github.com/gsoultan/cronos/internal/app/run"
)

// Workbooks adapts the use case's sheets to this writer.
//
// The mapping is trivial and that is the point: run assembles what a
// spreadsheet should contain and knows nothing about OOXML, while this package
// knows OOXML and nothing about reports.
type Workbooks struct{}

// New returns the adapter.
func New() Workbooks { return Workbooks{} }

// Write renders the sheets as an .xlsx.
func (Workbooks) Write(w io.Writer, sheets []run.SheetData) error {
	book := Book{Sheets: make([]Sheet, 0, len(sheets))}
	for _, s := range sheets {
		sheet := Sheet{
			Name: s.Name, Headers: s.Headers,
			FreezeHeader: s.FreezeHeader, AutoFilter: s.AutoFilter,
			Rows: make([][]Cell, 0, len(s.Rows)),
		}
		for _, row := range s.Rows {
			cells := make([]Cell, len(row))
			for i, v := range row {
				if v.Numeric {
					cells[i] = Num(v.Number)
					continue
				}
				cells[i] = Str(v.Text)
			}
			sheet.Rows = append(sheet.Rows, cells)
		}
		book.Sheets = append(book.Sheets, sheet)
	}
	return Write(w, book)
}
