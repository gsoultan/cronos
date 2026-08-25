package paginated

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/document"
)

// A statement per customer, with one long enough to spill onto a second and
// third page. The spill is the point: single-page groups never exercise the
// running header, the repeated column headings, or the page counter.
func fixture(counts ...int) document.Document {
	doc := document.Document{
		Title:  "Monthly invoice statement",
		Period: "1–31 July 2026",
		Org:    document.Org{Name: "Acme Logistics"},
		Page:   document.Page{Size: "a4", MarginMM: 18},
		Columns: []document.Column{
			{Field: "number", Label: "Invoice", Align: "left"},
			{Field: "issued_at", Label: "Issued", Align: "left"},
			{Field: "status", Label: "Status", Align: "left"},
			{Field: "total", Label: "Amount", Align: "right"},
		},
	}
	for i, n := range counts {
		g := document.Group{
			Label:    fmt.Sprintf("Customer %02d", i),
			Address:  []string{"12 Harbour Road", "Rotterdam", "Netherlands"},
			Meta:     []document.Entry{{Key: "Account", Value: fmt.Sprintf("AC-%d", 4000+i)}},
			Subtotal: map[string]string{"total": "€123,456.78"},
		}
		for k := range n {
			g.Rows = append(g.Rows, []string{
				fmt.Sprintf("INV-2026-%02d%03d", i, k), "2026-07-14", "Sent", "€1,234.56",
			})
		}
		doc.Groups = append(doc.Groups, g)
	}
	return doc
}

func TestRenderProducesAPDF(t *testing.T) {
	var buf bytes.Buffer
	if err := New(TypstCLI{}).Render(context.Background(), fixture(3, 40), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := buf.Bytes(); !bytes.HasPrefix(got, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF, starts with %q", got[:min(8, len(got))])
	}
	if buf.Len() < 2000 {
		t.Errorf("PDF is %d bytes — too small to contain two statements", buf.Len())
	}
	keep(t, buf.Bytes())
}

// A template is a visual artefact and the assertions above cannot see it.
// CRONOS_PDF_OUT=/tmp/s.pdf go test ./... leaves one to open.
func keep(t *testing.T, pdf []byte) {
	t.Helper()
	path := os.Getenv("CRONOS_PDF_OUT")
	if path == "" {
		return
	}
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d KB)", path, len(pdf)/1024)
}

// The renderer must leave nothing behind. A burst is thousands of renders, so
// a leaked directory per render is a full disk by the end of the month.
//
// Asked as "did this render leave a root that was not already there", rather
// than by counting. The count was the same question when the temp directory
// holds nothing else, and a different one the moment it does: a render killed
// mid-flight never runs its deferred cleanup, so a machine that has ever had a
// burst interrupted carries stale roots, and any of them disappearing during
// this test — an OS temp sweep, another run finishing — failed it for a reason
// having nothing to do with the renderer. Comparing sets ignores what was
// already there and still catches the leak this exists to catch.
func TestRenderRemovesItsRoot(t *testing.T) {
	before := renderRoots(t)
	if err := New(TypstCLI{}).Render(context.Background(), fixture(2), &bytes.Buffer{}); err != nil {
		t.Fatalf("render: %v", err)
	}
	for root := range renderRoots(t) {
		if !before[root] {
			t.Errorf("render leaked its root: %s", root)
		}
	}
}

// TestRenderRootCheckIgnoresStrangers pins the property the count did not have.
//
// A stale root vanishing mid-test is somebody else's business. Without this the
// check is red on any machine with leftovers, which is every machine where a
// burst has ever been interrupted — and a test that cries wolf about a full
// disk is one people stop reading.
func TestRenderRootCheckIgnoresStrangers(t *testing.T) {
	stranger, err := os.MkdirTemp("", "cronos-render-*")
	if err != nil {
		t.Fatal(err)
	}
	before := renderRoots(t)
	if !before[stranger] {
		t.Fatalf("%s is not being seen as a render root", stranger)
	}
	// Gone before the second look, the way a temp sweep takes one.
	if err := os.RemoveAll(stranger); err != nil {
		t.Fatal(err)
	}
	for root := range renderRoots(t) {
		if !before[root] {
			t.Errorf("a stranger disappearing was read as a leak: %s", root)
		}
	}
}

// Only the two staged files are reachable. Typst confines paths to --root, so
// what is in the root is exactly the document's attack surface.
func TestStageWritesOnlyTheDocument(t *testing.T) {
	dir := t.TempDir()
	if err := New(TypstCLI{}).stage(dir, fixture(1)); err != nil {
		t.Fatalf("stage: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{dataFile, mainFile}; strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("render root holds %v, want exactly %v", names, want)
	}
}

// Page X of Y counts the recipient's statement, not the document. A customer
// receiving three pages must see "of 3" even though they are pages 4 to 6 of
// the run.
func TestPageNumbersAreScopedToEachStatement(t *testing.T) {
	spans := statementSpans(t, fixture(3, 3, 90, 3))

	if len(spans) != 4 {
		t.Fatalf("got %d statements, want 4", len(spans))
	}
	if spans[2].pages < 2 {
		t.Fatalf("the 90-row statement fits one page (%d) — fixture no longer tests a spill",
			spans[2].pages)
	}
	if spans[3].start <= spans[2].start {
		t.Errorf("statement 4 starts on page %d, not after statement 3 on %d",
			spans[3].start, spans[2].start)
	}
	for i, s := range spans {
		if s.pages < 1 {
			t.Errorf("statement %d spans %d pages", i, s.pages)
		}
	}
	// Every statement begins a page of its own: pageBreak perGroup.
	for i := 1; i < len(spans); i++ {
		if spans[i].start != spans[i-1].start+spans[i-1].pages {
			t.Errorf("statement %d starts on page %d, but %d ended on %d — a page is shared",
				i, spans[i].start, i-1, spans[i-1].start+spans[i-1].pages-1)
		}
	}
}

func TestMissingBinaryIsItsOwnError(t *testing.T) {
	err := New(TypstCLI{Bin: "typst-does-not-exist"}).
		Render(context.Background(), fixture(1), &bytes.Buffer{})
	if !errors.Is(err, ErrTypstMissing) {
		t.Errorf("got %v, want ErrTypstMissing", err)
	}
}

type span struct{ start, pages int }

// statementSpans asks the compiled document where each statement began and
// ended. Typst's own introspection, not PDF text extraction: the assertion is
// then about the layout the template produced rather than about font
// subsetting and glyph encodings.
func statementSpans(t *testing.T, doc document.Document) []span {
	t.Helper()
	dir := t.TempDir()
	if err := New(TypstCLI{}).stage(dir, doc); err != nil {
		t.Fatal(err)
	}
	starts := queryPages(t, dir, "stmt-start")
	ends := queryPages(t, dir, "stmt-end")
	if len(starts) != len(ends) {
		t.Fatalf("%d starts but %d ends — the template lost a marker", len(starts), len(ends))
	}
	spans := make([]span, len(starts))
	for i := range starts {
		spans[i] = span{start: starts[i], pages: ends[i] - starts[i] + 1}
	}
	return spans
}

func queryPages(t *testing.T, dir, label string) []int {
	t.Helper()
	expr := fmt.Sprintf("query(<%s>).map(m => m.location().page())", label)
	cmd := exec.Command(TypstCLI{}.bin(), "eval", "--root", dir, "--in", mainFile, expr)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("typst eval %s: %v: %s", label, err, stderrOf(err))
	}
	var pages []int
	if err := json.Unmarshal(out, &pages); err != nil {
		t.Fatalf("typst eval %s returned %q: %v", label, out, err)
	}
	return pages
}

func stderrOf(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return string(ee.Stderr)
	}
	return ""
}

// renderRoots is the set of render roots currently on disk.
func renderRoots(t *testing.T) map[string]bool {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "cronos-render-*"))
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]bool, len(matches))
	for _, m := range matches {
		out[m] = true
	}
	return out
}
