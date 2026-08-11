package run_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/adapter/render/paginated"
	"github.com/gsoultan/cronos/internal/adapter/render/spreadsheet"
	"github.com/gsoultan/cronos/internal/app/run"
)

/*
 * A report's spreadsheet output, end to end and opened by a real reader.
 *
 * The claim worth checking is not that XLSX is produced — write_test.go covers
 * the format. It is that the amounts arrive as *numbers* after passing through
 * a query, a driver and two mapping layers, because the formatted path this
 * project uses everywhere else would have turned them into strings and nobody
 * would notice until a recipient tried to sum a column.
 */

const spreadsheetReport = `
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: export}
spec:
  dataset: invoices
  outputs:
    - name: xlsx
      renderer: spreadsheet
      sheets:
        - name: Invoices
          columns: [customer_name, issued_at, status, total]
          freezeHeader: true
          autoFilter: true
`

func TestASpreadsheetExportHoldsNumbers(t *testing.T) {
	python := os.Getenv("CRONOS_XLSX_PYTHON")
	if python == "" {
		t.Skip("set CRONOS_XLSX_PYTHON to a python with openpyxl")
	}

	svc, _ := setup(t)
	report, err := yamlcodec.Loader{}.Report([]byte(spreadsheetReport))
	if err != nil {
		t.Fatal(err)
	}

	statements := run.NewStatements(svc, paginated.New(paginated.TypstCLI{})).
		WithWorkbooks(spreadsheet.New())

	xlsx, err := statements.Spreadsheet(context.Background(), report, "xlsx",
		map[string]any{"from": "2026-01-01"}, customer("c-1"))
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	path := filepath.Join(t.TempDir(), "export.xlsx")
	if err := os.WriteFile(path, xlsx, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(python, "-c", `
import json, sys, openpyxl
ws = openpyxl.load_workbook(sys.argv[1]).worksheets[0]
print(json.dumps({
  "rows": [[str(v) if v is not None else None for v in r] for r in ws.iter_rows(values_only=True)],
  "types": [[type(v).__name__ for v in r] for r in ws.iter_rows(values_only=True)],
  "freeze": ws.freeze_panes,
}))`, path).Output()
	if err != nil {
		t.Fatalf("openpyxl refused the export: %v", err)
	}

	var got struct {
		Rows   [][]any    `json:"rows"`
		Types  [][]string `json:"types"`
		Freeze *string    `json:"freeze"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}

	// The heading row plus c-1's three invoices, and nobody else's.
	if len(got.Rows) != 4 {
		t.Fatalf("got %d rows: %v", len(got.Rows), got.Rows)
	}
	// Labels from the dataset, not column names.
	if got.Rows[0][0] != "Customer" || got.Rows[0][3] != "Amount" {
		t.Errorf("headings = %v", got.Rows[0])
	}
	// The whole reason for the second, unformatted path.
	if kind := got.Types[1][3]; kind != "float" && kind != "int" {
		t.Errorf("an amount exported as %s — a recipient cannot sum that", kind)
	}
	// And a date stays text rather than becoming a serial number nobody reads.
	if got.Types[1][1] != "str" {
		t.Errorf("a date exported as %s", got.Types[1][1])
	}
	if got.Freeze == nil || *got.Freeze != "A2" {
		t.Errorf("freeze = %v", got.Freeze)
	}
}

// Row scope reaches an export exactly as it reaches everything else.
func TestAnExportIsScopedToTheCaller(t *testing.T) {
	python := os.Getenv("CRONOS_XLSX_PYTHON")
	if python == "" {
		t.Skip("set CRONOS_XLSX_PYTHON to a python with openpyxl")
	}

	svc, _ := setup(t)
	report, _ := yamlcodec.Loader{}.Report([]byte(spreadsheetReport))
	statements := run.NewStatements(svc, paginated.New(paginated.TypstCLI{})).
		WithWorkbooks(spreadsheet.New())

	one, err := statements.Spreadsheet(context.Background(), report, "xlsx",
		map[string]any{"from": "2026-01-01"}, customer("c-1"))
	if err != nil {
		t.Fatal(err)
	}
	two, err := statements.Spreadsheet(context.Background(), report, "xlsx",
		map[string]any{"from": "2026-01-01"}, customer("c-2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(one) == len(two) {
		t.Error("two customers exported byte-identical files")
	}
}
