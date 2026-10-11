package run_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/adapter/render/paginated"
	"github.com/gsoultan/cronos/internal/adapter/render/spreadsheet"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
)

const pivotReport = `
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: crossed}
spec:
  dataset: invoices
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: pivot
          title: Billed by status and month
          value: {field: total}
          pivot:
            rows: {field: status}
            columns: {field: issued_at, grain: month}
    - name: xlsx
      renderer: spreadsheet
      sheets:
        - name: By month
          value: {field: total}
          pivot:
            rows: {field: status}
            columns: {field: issued_at, grain: month}
`

func crossedReport(t *testing.T) definition.Report {
	t.Helper()
	rep, err := yamlcodec.Loader{}.Report([]byte(pivotReport))
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

// Row scope reaches a pivot's cells and its totals: c-1 sees its own 4,000 and
// none of Baltic's 13,000, and a customer with no invoices sees no rows at all
// rather than somebody else's.
func TestAPivotIsScopedToTheCaller(t *testing.T) {
	svc, _ := setup(t)
	rep := crossedReport(t)

	got := pivotRows(render(t, svc, rep, run.Request{Output: "interactive"}, customer("c-1")).Blocks[0])
	want := [][]string{
		{"Status", "Jul 2026", "Aug 2026", "Total"},
		{"overdue", "2,500", "", "2,500"},
		{"paid", "", "500", "500"},
		{"sent", "1,000", "", "1,000"},
		{"Total", "3,500", "500", "4,000"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("c-1's pivot\n got %q\nwant %q", got, want)
	}

	nobody := render(t, svc, rep, run.Request{Output: "interactive"}, customer("c-9")).Blocks[0]
	if len(nobody.Rows) != 0 {
		t.Errorf("a customer with no invoices read %q", nobody.Rows)
	}
}

// A pivot tab's cells and totals are numbers a recipient can add up again.
func TestAPivotSheetHoldsNumbers(t *testing.T) {
	python := os.Getenv("CRONOS_XLSX_PYTHON")
	if python == "" {
		t.Skip("set CRONOS_XLSX_PYTHON to a python with openpyxl")
	}
	svc, _ := setup(t)
	statements := run.NewStatements(svc, paginated.New(paginated.TypstCLI{})).
		WithWorkbooks(spreadsheet.New())
	xlsx, err := statements.Spreadsheet(context.Background(), crossedReport(t), "xlsx",
		map[string]any{"from": "2026-01-01"}, customer("c-1"))
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	path := filepath.Join(t.TempDir(), "pivot.xlsx")
	if err := os.WriteFile(path, xlsx, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(python, "-c", `
import json, sys, openpyxl
ws = openpyxl.load_workbook(sys.argv[1]).worksheets[0]
print(json.dumps([[v for v in r] for r in ws.iter_rows(values_only=True)]))`, path).Output()
	if err != nil {
		t.Fatalf("openpyxl refused the export: %v", err)
	}
	var rows [][]any
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 || !reflect.DeepEqual(rows[0], []any{"Status", "Jul 2026", "Aug 2026", "Total"}) {
		t.Fatalf("sheet %v", rows)
	}
	// The grand total, as a number.
	if total, ok := rows[4][3].(float64); !ok || total != 4000 {
		t.Errorf("grand total %#v, want the number 4000", rows[4][3])
	}
}
