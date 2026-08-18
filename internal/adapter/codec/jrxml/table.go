package jrxml

import (
	"sort"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
The table is inferred, and this is the inference.

It is the one place the import reads a layout rather than a declaration, so it
is worth stating what it rests on. A JasperReports detail band is drawn once per
row of the result set. Every tabular report ever written therefore places one
text field per column in that band, side by side; the column header band above
it places a static label over each. Nothing in the file says "this is a table" —
but a band repeated per row, holding N field references at N x-positions, is a
table, and reading it as one recovers the columns and their order.

What that cannot recover is anything the coordinates do not encode. A detail
band with two rows of fields is two lines per record, which cronos has no way to
draw; a field positioned by a `printWhenExpression` may or may not be a column.
Both are reported.
*/

// columns reads the detail band as a table's columns, in page order.
func (t *translation) columns() []string {
	placed := t.doc.Detail.fields()
	if len(placed) == 0 {
		return nil
	}
	sortReading(placed)

	if rows := distinctRows(placed); rows > 1 {
		// Two stacked lines per record. The columns come out as one flat list,
		// which is a different document from the original.
		t.found.addf(Review, "detail",
			"the detail band draws %d rows per record; a cronos table draws one, so the fields were flattened into a single row of columns",
			rows)
	}

	var out []string
	for _, p := range placed {
		name, ok := t.columnOf(p)
		if !ok {
			continue
		}
		if contains(out, name) {
			// The same column twice in one band is a Jasper layout trick — the
			// same value repeated in two places — and a duplicate column in a
			// cronos table is just a duplicate.
			continue
		}
		out = append(out, name)
	}
	return out
}

// columnOf reads one detail text field as a column name.
func (t *translation) columnOf(p placedField) (string, bool) {
	if p.field.Element.PrintWhenExpression != "" {
		// Reported by the census as well; here it says which column.
		t.found.addf(Review, "printWhenExpression",
			"a detail column was drawn conditionally and is imported unconditionally: %s",
			oneLine(p.field.Expression))
	}
	if p.field.EvaluationTime != "" && p.field.EvaluationTime != "Now" {
		t.found.addf(Review, "textField",
			"a detail column was evaluated at %s rather than per row, so it showed a total where a value now appears: %s",
			p.field.EvaluationTime, oneLine(p.field.Expression))
	}

	r, plain := plainRef(p.field.Expression)
	if !plain {
		t.found.addf(Review, "textFieldExpression",
			"a detail column is the Java expression %s; cronos has no computed column, so it is missing — add it to the dataset's SELECT",
			oneLine(p.field.Expression))
		return "", false
	}
	switch r.sigil {
	case 'F':
		name, known := t.fieldNames[r.name]
		if !known {
			t.found.addf(Review, "textFieldExpression",
				"a detail column reads $F{%s}, which the report does not declare as a field", r.name)
			return "", false
		}
		return name, true
	case 'V':
		t.found.addf(Review, "variable",
			"a detail column shows the running variable $V{%s}; cronos tables have no running column, and a per-group total is a subtotal instead",
			r.name)
	case 'P':
		t.found.addf(Note, "textFieldExpression",
			"a detail column repeats the parameter $P{%s} on every row, which a cronos table does not do",
			r.name)
	}
	return "", false
}

// label reads the column header band for what each column was called.
//
// Matched by horizontal overlap rather than by index: a header band routinely
// holds fewer labels than the detail holds fields — a spacer column, a merged
// heading over two — and pairing them by position is what a reader does.
func (t *translation) labels() map[string]string {
	headers := t.doc.ColumnHeader.texts()
	if len(headers) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, p := range t.doc.Detail.fields() {
		name, ok := t.columnOfQuiet(p)
		if !ok {
			continue
		}
		if text := bestOverlap(p, headers); text != "" {
			out[name] = text
		}
	}
	return out
}

// columnOfQuiet resolves a column without recording findings, for the second
// pass that only wants labels.
func (t *translation) columnOfQuiet(p placedField) (string, bool) {
	r, plain := plainRef(p.field.Expression)
	if !plain || r.sigil != 'F' {
		return "", false
	}
	name, known := t.fieldNames[r.name]
	return name, known
}

// bestOverlap returns the header text sharing the most width with the field.
func bestOverlap(p placedField, headers []placedText) string {
	best, bestWidth := "", 0
	for _, h := range headers {
		width := overlap(p.x, p.x+p.field.Element.Width, h.x, h.x+h.text.Element.Width)
		if width > bestWidth {
			best, bestWidth = strings.TrimSpace(h.text.Text), width
		}
	}
	// A label has to actually sit over the column. A header sharing one pixel
	// with it is the neighbouring column's.
	if bestWidth*4 < p.field.Element.Width {
		return ""
	}
	return collapse(best)
}

func overlap(aLo, aHi, bLo, bHi int) int {
	lo, hi := max(aLo, bLo), min(aHi, bHi)
	if hi <= lo {
		return 0
	}
	return hi - lo
}

// grouping reads the report's groups as a table's groupBy, page break and
// subtotals.
func (t *translation) grouping(columns []string) (groupBy, pageBreak string, subtotals []string) {
	if len(t.doc.Groups) == 0 {
		return "", "", nil
	}
	first := t.doc.Groups[0]
	if len(t.doc.Groups) > 1 {
		var rest []string
		for _, g := range t.doc.Groups[1:] {
			rest = append(rest, g.Name)
		}
		t.found.addf(Review, "group",
			"a cronos table groups by one field; %q was kept and the nested %s %s not imported",
			first.Name, strings.Join(rest, ", "), plural(len(rest), "was", "were"))
	}

	r, plain := plainRef(first.Expression)
	if !plain || r.sigil != 'F' {
		t.found.addf(Review, "groupExpression",
			"group %q breaks on the Java expression %s; cronos groups by a field, so the grouping is not imported",
			first.Name, oneLine(first.Expression))
		return "", "", nil
	}
	groupBy, known := t.fieldNames[r.name]
	if !known {
		t.found.addf(Review, "groupExpression",
			"group %q breaks on $F{%s}, which the report does not declare as a field", first.Name, r.name)
		return "", "", nil
	}
	// A grouped table has to select the column it groups by, or the renderer
	// has a heading with nothing in it.
	if !contains(columns, groupBy) {
		t.found.addf(Note, "group",
			"the table groups by %q, which the detail band did not draw; it is added as a column so the group heading has a value",
			groupBy)
	}

	if first.StartNewPage {
		pageBreak = "perGroup"
	}
	if first.ReprintHeaderOnEachPage != nil && !*first.ReprintHeaderOnEachPage {
		t.found.add(Note, "group",
			"the original did not repeat the group heading on a page break; a cronos grouped table always does")
	}
	return groupBy, pageBreak, t.subtotals(first)
}

// subtotals reads the per-group variables as a table's subtotals.
func (t *translation) subtotals(g group) []string {
	var out []string
	for _, v := range t.doc.Variables {
		if !strings.EqualFold(v.ResetType, "Group") || v.ResetGroup != g.Name {
			continue
		}
		r, plain := plainRef(v.Expression)
		if !plain || r.sigil != 'F' {
			t.found.addf(Review, "variable",
				"the per-group total %q is the Java expression %s, which cronos cannot subtotal",
				v.Name, oneLine(v.Expression))
			continue
		}
		if _, ok := aggregateOf(v.Calculation); !ok {
			t.found.addf(Review, "variable",
				"the per-group total %q calculates %s, which is not a cronos aggregate — the subtotal is not imported",
				v.Name, orNothing(v.Calculation))
			continue
		}
		name, known := t.fieldNames[r.name]
		if !known || contains(out, name) {
			continue
		}
		out = append(out, name)
	}
	return out
}

// sortKeys reads the report's sort fields.
func (t *translation) sortKeys() []definition.SortKey {
	var out []definition.SortKey
	for _, s := range t.doc.SortFields {
		if s.Type != "" && !strings.EqualFold(s.Type, "Field") {
			t.found.addf(Review, "sortField",
				"the report sorted on the variable %q; cronos sorts on fields, so it is not imported", s.Name)
			continue
		}
		name, known := t.fieldNames[s.Name]
		if !known {
			t.found.addf(Review, "sortField",
				"the report sorted on %q, which it does not declare as a field", s.Name)
			continue
		}
		dir := ""
		if strings.EqualFold(s.Order, "Descending") {
			dir = "desc"
		}
		out = append(out, definition.SortKey{Field: name, Dir: dir})
	}
	return out
}

// sortReading orders placed elements the way a page is read: down, then across.
//
// Tolerant on y, because a designer nudging one field two pixels to line up a
// baseline did not mean to start a new row, and an exact sort would put that
// column last.
func sortReading[T interface{ pos() (int, int) }](items []T) {
	sort.SliceStable(items, func(i, j int) bool {
		xi, yi := items[i].pos()
		xj, yj := items[j].pos()
		if sameRow(yi, yj) {
			return xi < xj
		}
		return yi < yj
	})
}

// rowTolerance is how far apart two elements can sit vertically and still be one
// row. A text field is around 20 points tall in every Jasper report; half of that
// is a nudge and more of it is a second line.
const rowTolerance = 8

func sameRow(a, b int) bool {
	if a > b {
		a, b = b, a
	}
	return b-a <= rowTolerance
}

func (p placedField) pos() (int, int) { return p.x, p.y }
func (p placedText) pos() (int, int)  { return p.x, p.y }

// distinctRows counts how many bands of y the fields occupy.
func distinctRows(placed []placedField) int {
	rows := 0
	last := 0
	for i, p := range placed {
		if i == 0 || !sameRow(last, p.y) {
			rows++
			last = p.y
		}
	}
	return rows
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// collapse flattens the newlines Jasper Studio leaves in a wrapped heading.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func orNothing(s string) string {
	if strings.TrimSpace(s) == "" {
		return "nothing"
	}
	return s
}
