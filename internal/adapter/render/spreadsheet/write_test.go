package spreadsheet

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * Every file this package produces is opened with openpyxl.
 *
 * Hand-writing OOXML is only a reasonable trade because it is checked against
 * a real reader rather than against my reading of the specification. "It
 * probably parses" would not be worth the fifteen megabytes it saves.
 *
 * Set CRONOS_XLSX_PYTHON to a python with openpyxl installed. Without it these
 * skip — and skipping is called out in the failure message rather than left to
 * look like a pass, because a renderer nobody verified is the thing this file
 * exists to prevent.
 */

const reader = `
import json, sys, openpyxl
wb = openpyxl.load_workbook(sys.argv[1])
out = []
for ws in wb.worksheets:
    out.append({
        "name": ws.title,
        "rows": [[None if v is None else v for v in row]
                 for row in ws.iter_rows(values_only=True)],
        "types": [[type(v).__name__ for v in row] for row in ws.iter_rows(values_only=True)],
        "freeze": ws.freeze_panes,
        "autofilter": ws.auto_filter.ref,
    })
print(json.dumps(out))
`

type readSheet struct {
	Name       string     `json:"name"`
	Rows       [][]any    `json:"rows"`
	Types      [][]string `json:"types"`
	Freeze     *string    `json:"freeze"`
	AutoFilter *string    `json:"autofilter"`
}

// open writes the workbook and reads it back with openpyxl.
func open(t *testing.T, b Book) []readSheet {
	t.Helper()
	python := os.Getenv("CRONOS_XLSX_PYTHON")
	if python == "" {
		t.Skip("set CRONOS_XLSX_PYTHON to a python with openpyxl — " +
			"these assertions are the reason hand-written OOXML is acceptable")
	}

	path := filepath.Join(t.TempDir(), "book.xlsx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(f, b); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	cmd := exec.Command(python, "-c", reader, path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("openpyxl refused the file: %v\n%s", err, stderr.String())
	}

	var sheets []readSheet
	if err := json.Unmarshal(out, &sheets); err != nil {
		t.Fatal(err)
	}
	return sheets
}

func invoices() Book {
	return Book{Sheets: []Sheet{{
		Name:    "Invoices",
		Headers: []string{"Customer", "Issued", "Status", "Amount"},
		Rows: [][]Cell{
			{Str("Aurora Freight"), Str("2026-07-04"), Str("sent"), Num(1200.50)},
			{Str("Cedar & Vine <Foods>"), Str("2026-07-19"), Str("overdue"), Num(-99)},
		},
		FreezeHeader: true, AutoFilter: true,
	}}}
}

// The entire reason somebody asked for a spreadsheet rather than a PDF: a
// total written as text is a total they cannot sum.
func TestNumbersArriveAsNumbers(t *testing.T) {
	sheets := open(t, invoices())

	if len(sheets) != 1 || sheets[0].Name != "Invoices" {
		t.Fatalf("sheets = %+v", sheets)
	}
	amounts := sheets[0].Types
	if got := amounts[1][3]; got != "float" && got != "int" {
		t.Errorf("an amount came back as %s", got)
	}
	if sheets[0].Rows[1][3] != 1200.50 {
		t.Errorf("amount = %v", sheets[0].Rows[1][3])
	}
	// And the headings stay text.
	if amounts[0][0] != "str" {
		t.Errorf("a heading came back as %s", amounts[0][0])
	}
}

// An ampersand in a company name should not produce a file nobody can open.
func TestCustomerDataSurvivesTheXML(t *testing.T) {
	sheets := open(t, invoices())
	if got := sheets[0].Rows[2][0]; got != "Cedar & Vine <Foods>" {
		t.Errorf("name came back as %q", got)
	}
}

// The two things every recipient does by hand the moment they open the file.
func TestFreezeAndAutoFilterAreApplied(t *testing.T) {
	sheets := open(t, invoices())

	if sheets[0].Freeze == nil || *sheets[0].Freeze != "A2" {
		t.Errorf("freeze = %v, want the header row held", sheets[0].Freeze)
	}
	// The filter must cover the rows, not only the headings, or the dropdown
	// labels the table without filtering it.
	if sheets[0].AutoFilter == nil || *sheets[0].AutoFilter != "A1:D3" {
		t.Errorf("autofilter = %v, want A1:D3", sheets[0].AutoFilter)
	}
}

// A NUL in a database column would otherwise produce a file every reader
// rejects, and the report would look broken rather than the data.
func TestControlCharactersAreRemoved(t *testing.T) {
	sheets := open(t, Book{Sheets: []Sheet{{
		Name: "Odd", Headers: []string{"Name"},
		Rows: [][]Cell{{Str("Acme\x00Ltd\x1f")}},
	}}})

	if got := sheets[0].Rows[1][0]; got != "AcmeLtd" {
		t.Errorf("value = %q", got)
	}
}

// Excel forbids five characters in a sheet name and caps it at 31. A name that
// breaks either produces a file that will not open, which reads as a corrupt
// download rather than a naming mistake.
func TestSheetNamesAreMadeAcceptable(t *testing.T) {
	sheets := open(t, Book{Sheets: []Sheet{
		{Name: "Q1/Q2 [draft]", Headers: []string{"a"}},
		{Name: strings.Repeat("long ", 20), Headers: []string{"a"}},
		{Name: "  ", Headers: []string{"a"}},
	}})

	if strings.ContainsAny(sheets[0].Name, `:\/?*[]`) {
		t.Errorf("name = %q still has a forbidden character", sheets[0].Name)
	}
	if len([]rune(sheets[1].Name)) > 31 {
		t.Errorf("name is %d characters", len([]rune(sheets[1].Name)))
	}
	if strings.TrimSpace(sheets[2].Name) == "" {
		t.Error("a blank name should become something")
	}
}

func TestSeveralSheets(t *testing.T) {
	sheets := open(t, Book{Sheets: []Sheet{
		{Name: "Invoices", Headers: []string{"a"}, Rows: [][]Cell{{Str("x")}}},
		{Name: "Credits", Headers: []string{"b"}, Rows: [][]Cell{{Num(3)}}},
	}})

	if len(sheets) != 2 {
		t.Fatalf("got %d sheets", len(sheets))
	}
	if sheets[1].Name != "Credits" || sheets[1].Rows[1][0] != float64(3) {
		t.Errorf("second sheet = %+v", sheets[1])
	}
}

// Column names are bijective base-26: after Z comes AA, not BA. Getting it
// wrong is invisible until a report has twenty-seven columns.
func TestColumnNames(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 51: "AZ", 52: "BA", 701: "ZZ", 702: "AAA"} {
		if got := column(i); got != want {
			t.Errorf("column(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestAWorkbookWithNoSheetsIsRefused(t *testing.T) {
	// Excel refuses it, and to a recipient that reads as a corrupt download
	// rather than an empty report.
	if err := Write(&bytes.Buffer{}, Book{}); err == nil {
		t.Error("wrote a workbook with no sheets")
	}
}

// Twenty-seven columns, so the bijective base-26 is exercised through a real
// reader rather than only in a unit test.
func TestAWideSheetOpens(t *testing.T) {
	headers := make([]string, 30)
	row := make([]Cell, 30)
	for i := range headers {
		headers[i] = column(i)
		row[i] = Num(float64(i))
	}
	sheets := open(t, Book{Sheets: []Sheet{{Name: "Wide", Headers: headers, Rows: [][]Cell{row}}}})

	if got := len(sheets[0].Rows[0]); got != 30 {
		t.Fatalf("got %d columns", got)
	}
	if sheets[0].Rows[1][26] != float64(26) {
		t.Errorf("column AA holds %v", sheets[0].Rows[1][26])
	}
}
