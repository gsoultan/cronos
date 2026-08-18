package jrxml

import (
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// paper is the page sizes cronos names, in PostScript points.
//
// Points because that is the only unit a `.jrxml` measures in: pageWidth="595"
// is A4 and nothing in the file says so. Matched by dimension rather than
// trusted from a property, since Jasper Studio writes the property only when a
// person picked the size from its menu, and a report that has been through three
// authors usually has bare numbers.
var paper = []struct {
	name          string
	width, height int
}{
	{"A4", 595, 842},
	{"A3", 842, 1191},
	{"A5", 420, 595},
	{"Letter", 612, 792},
	{"Legal", 612, 1008},
	{"us-tabloid", 792, 1224},
}

// paperTolerance absorbs the rounding between millimetres and points. A4 is
// 595.28pt, and files round it down, up, or to 596.
const paperTolerance = 4

// page translates the paper the report printed on.
func (t *translation) page() definition.PageSpec {
	out := definition.PageSpec{
		Orientation: t.orientation(),
		Margins:     t.margins(),
	}
	out.Size = t.paperSize()
	return out
}

func (t *translation) paperSize() string {
	w, h := t.doc.PageWidth, t.doc.PageHeight
	if w == 0 || h == 0 {
		// A file with no page dimensions is one the renderer should default,
		// rather than one printed on a zero-sized sheet.
		return ""
	}
	for _, p := range paper {
		// Either way round: a landscape report states the rotated dimensions
		// and says so in its orientation attribute.
		if near(w, p.width) && near(h, p.height) || near(w, p.height) && near(h, p.width) {
			return p.name
		}
	}
	t.found.addf(Review, "jasperReport",
		"the report printed on a %d×%d point page, which is not a paper cronos names — the output uses the renderer's default, so set page.size if it mattered",
		w, h)
	return ""
}

func (t *translation) orientation() string {
	if strings.EqualFold(t.doc.Orientation, "Landscape") {
		return "landscape"
	}
	return "portrait"
}

// margins collapses Jasper's four margins into the one cronos has.
//
// The largest, not the first: a cronos margin applies to every side, and
// choosing the smallest would push content past the edge the original kept
// clear. Reported when they differ, because a report designed with a wide left
// binding margin will now have it on all four sides.
func (t *translation) margins() string {
	l, r := t.doc.LeftMargin, t.doc.RightMargin
	top, bottom := t.doc.TopMargin, t.doc.BottomMargin
	widest := max(max(l, r), max(top, bottom))
	if widest == 0 {
		return ""
	}
	if l != r || top != bottom || l != top {
		t.found.addf(Note, "jasperReport",
			"the page had margins of %d/%d/%d/%d points; cronos takes one margin for every side, so the widest was used",
			top, r, bottom, l)
	}
	return millimetres(widest)
}

// millimetres formats points as the "20mm" the format writes.
//
// One decimal, trimmed. run.millimetres parses a float happily, and 7.1mm is
// the truth about a 20-point margin where 7mm is a redesign.
func millimetres(points int) string {
	mm := float64(points) * 25.4 / 72
	s := fmt.Sprintf("%.1f", mm)
	s = strings.TrimSuffix(s, ".0")
	return s + "mm"
}

func near(a, b int) bool {
	if a > b {
		a, b = b, a
	}
	return b-a <= paperTolerance
}

// footer translates the page footer.
//
// Only the page number really crosses. A Jasper footer is a band of positioned
// elements and a cronos footer is one line of text, so the translation is
// deliberately narrow: the running "Page 1 of 12" that almost every report has,
// or a single static line if that is all there is.
func (t *translation) footer() definition.Furniture {
	band := t.doc.PageFooter
	if band.empty() {
		band = t.doc.LastPageFooter
	}
	if band.empty() {
		return definition.Furniture{}
	}

	number, total := false, false
	for _, f := range band.fields() {
		for _, r := range scanRefs(f.field.Expression) {
			if r.sigil != 'V' {
				continue
			}
			switch r.name {
			case "PAGE_NUMBER":
				number = true
			case "PAGE_COUNT":
				total = true
			}
		}
	}

	switch {
	case number && total:
		return definition.Furniture{Text: "Page {{ .page }} of {{ .pages }}"}
	case number:
		return definition.Furniture{Text: "Page {{ .page }}"}
	}

	texts := band.texts()
	if len(texts) == 1 {
		return definition.Furniture{Text: collapse(texts[0].text.Text)}
	}
	t.found.add(Review, "pageFooter",
		"the page footer drew something other than a page number; a cronos footer is one line of text, so it is not imported — a Typst template in footer.template can hold the original")
	return definition.Furniture{}
}

// title reads the report's own heading out of the title band.
//
// The first static text in reading order. A title band also holds the run date,
// a logo and a subtitle, and picking the largest font would need the font
// metrics this deliberately does not read — whereas the first thing on the page
// is the heading in every report that has one.
func (t *translation) title() string {
	texts := t.doc.Title.texts()
	if len(texts) == 0 {
		return ""
	}
	sortReading(texts)

	head := collapse(texts[0].text.Text)
	if rest := len(texts) - 1; rest > 0 {
		t.found.addN(Note, "title",
			"the title band held more than a heading; only the first line was imported as a text block",
			rest)
	}
	if fields := t.doc.Title.fields(); len(fields) > 0 {
		t.found.addN(Review, "title",
			"the title band printed a value — a run date or a parameter — which a cronos text block does not do",
			len(fields))
	}
	return head
}
