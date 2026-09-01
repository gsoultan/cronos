package run_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/app/run"
)

/*
An output that is a file, asked for as a view.

A report can carry several outputs and a caller names the one it wants. Two of
them draw blocks; a spreadsheet describes sheets instead, and has no layout at
all. Render walks the layout, so asking for the spreadsheet walked an empty
list and answered two hundred with a title and no blocks.

Nothing about that is visible from either end. A host application integrating
against the API gets a successful response and an empty report, and the reader
in front of it gets a page with nothing on it — the failure mode this project
keeps finding, where the wrong answer is indistinguishable from a quiet day.

Statements refuses an output that is not paginated and Workbook refuses one
that is not a spreadsheet, both by name. This was the third door.
*/

const threeOutputs = `
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: export}
spec:
  dataset: invoices
  outputs:
    - name: screen
      renderer: interactive
      layout:
        - kind: table
          columns: [customer_name, total]
    - name: xlsx
      renderer: spreadsheet
      sheets:
        - name: Invoices
          columns: [customer_name, total]
`

func TestAskingForASpreadsheetAsAViewIsRefused(t *testing.T) {
	svc, _ := setup(t)
	report, err := yamlcodec.Loader{}.Report([]byte(threeOutputs))
	if err != nil {
		t.Fatal(err)
	}
	req := func(out string) run.Request {
		return run.Request{Output: out, Params: map[string]any{"from": "2026-01-01"}}
	}

	// The one that draws still draws, so this refuses the right thing.
	view, err := svc.Render(context.Background(), report, req("screen"), customer("c-1"))
	if err != nil {
		t.Fatalf("the interactive output no longer renders: %v", err)
	}
	if len(view.Blocks) == 0 {
		t.Fatal("the interactive output rendered no blocks")
	}

	_, err = svc.Render(context.Background(), report, req("xlsx"), customer("c-1"))
	if err == nil {
		t.Fatal("a spreadsheet output rendered as a view — a caller gets 200 and an empty report")
	}
	if !errors.Is(err, run.ErrNotRenderable) {
		t.Fatalf("refused with %v, which the API does not map to a status", err)
	}
	// Named, because "cannot render" sends somebody to read the definition to
	// work out which of its outputs was the problem.
	for _, want := range []string{"xlsx", "spreadsheet"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %v", want, err)
		}
	}
}
