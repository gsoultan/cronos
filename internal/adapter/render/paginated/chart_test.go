package paginated

import (
	"bytes"
	"context"
	"math"
	"testing"

	"github.com/gsoultan/cronos/internal/core/document"
)

/*
 * Charts on a page.
 *
 * A paginated output used to drop them: a report with charts produced a PDF
 * that simply did not have them, and the format documentation claimed they
 * came out as static images. Nothing failed, so nothing said so.
 *
 * These compile real PDFs through the real typesetter, because the assertion
 * that matters is that Typst accepts the marks — a template that throws leaves
 * a burst with no output and a message about a subprocess.
 */

func marks() []document.Mark {
	// One of each primitive, which between them are every chart type: see
	// internal/core/document/mark.go.
	return []document.Mark{
		{Kind: document.RectMark, X: 0, Y: 0.1, W: 0.8, H: 0.2,
			Tone: "series-1", Label: "England", Value: "1,500"},
		{Kind: document.RectMark, X: 0, Y: 0.4, W: 0.35, H: 0.2,
			Tone: "down", Label: "Scotland", Value: "700"},
		{Kind: document.LineMark, Tone: "series-2",
			Points: [][2]float64{{0, 0.9}, {0.5, 0.3}, {1, 0.6}}},
		{Kind: document.PolyMark, Tone: "series-3-wash",
			Points: [][2]float64{{0, 1}, {0.5, 0.4}, {1, 0.7}, {1, 1}}},
		{Kind: document.DotMark, X: 0.3, Y: 0.5, W: 0.03, Tone: "series-4", Label: "A dot"},
	}
}

func charted() document.Document {
	doc := fixture(1, 3)
	doc.Charts = []document.Chart{
		{
			Title: "Every primitive", Kind: "bar", Marks: marks(),
			Ticks: []document.Tick{{At: 0, Label: "€0"}, {At: 1, Label: "€8.0M"}},
			Keys: []document.Key{
				{Tone: "series-1", Label: "Billed"},
				{Tone: "series-2", Label: "Margin (right)"},
			},
		},
		{
			// Square, because the points are a circle: drawn across the full
			// width it is an ellipse, and an ellipse encodes a direction the
			// data does not have.
			Title: "A pie", Kind: "pie", Square: true,
			Marks: []document.Mark{
				{Kind: document.PolyMark, Tone: "series-1", Points: wedge(0, 0.6)},
				{Kind: document.PolyMark, Tone: "series-2", Points: wedge(0.6, 0.4)},
			},
		},
		{
			// The case a page must not simply omit.
			Title: "A map", Kind: "map",
			Note: "Maps are not printed. Open this report in a browser to see it.",
		},
	}
	return doc
}

// wedge is the same polygon approximation run.slice produces, inlined so this
// package's test does not depend on that one's internals.
func wedge(from, span float64) [][2]float64 {
	pts := [][2]float64{}
	for i := 0; i <= 24; i++ {
		a := (from + span*float64(i)/24) * 2 * math.Pi
		pts = append(pts, [2]float64{0.5 + math.Cos(a)*0.5, 0.5 + math.Sin(a)*0.5})
	}
	return append(pts, [2]float64{0.5, 0.5})
}

func TestChartsAreTypesetOntoThePage(t *testing.T) {
	var buf bytes.Buffer
	if err := New(TypstCLI{}).Render(context.Background(), charted(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if got := buf.Bytes(); !bytes.HasPrefix(got, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF, starts with %q", got[:min(8, len(got))])
	}

	// A PDF carrying three charts is materially bigger than the same statement
	// without them; the marks are vector paths, and they have to be in there.
	var bare bytes.Buffer
	if err := New(TypstCLI{}).Render(context.Background(), fixture(1, 3), &bare); err != nil {
		t.Fatalf("render bare: %v", err)
	}
	if buf.Len() <= bare.Len() {
		t.Errorf("the charted PDF is %d bytes against %d bare — the marks did not reach the page",
			buf.Len(), bare.Len())
	}
	keep(t, buf.Bytes())
}

func TestAChartOnlyOutputIsADocument(t *testing.T) {
	// A layout of charts and no table is a dashboard on paper. It used to be
	// refused as "no table to print", which was the only honest answer while
	// charts were dropped before anything could see them.
	doc := document.Document{
		Title: "Dashboard", Period: "July",
		Org:    document.Org{Name: "Finance"},
		Page:   document.Page{Size: "a4", Orientation: "portrait", MarginMM: 18},
		Charts: []document.Chart{{Title: "Parcels", Kind: "bar", Marks: marks()}},
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("a chart-only document is a document: %v", err)
	}

	var buf bytes.Buffer
	if err := New(TypstCLI{}).Render(context.Background(), doc, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatal("a chart-only report produced no PDF")
	}
}

func TestAMarkNothingDrawsIsRefusedBeforeTypst(t *testing.T) {
	// Typst would accept the document and draw nothing, so the page would come
	// back missing a chart with no error anywhere.
	doc := charted()
	doc.Charts[0].Marks = append(doc.Charts[0].Marks, document.Mark{Kind: "sunburst"})
	if err := doc.Validate(); err == nil {
		t.Fatal("a mark kind nothing draws was accepted")
	}
}
