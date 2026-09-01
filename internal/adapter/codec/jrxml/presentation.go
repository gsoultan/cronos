package jrxml

import (
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
The three things about a Jasper report's appearance that are worth keeping.

Everything else in a band is geometry, and geometry does not survive the move to
a semantic format. But a column heading, a number's format and the caption
beside a total are not geometry — they are text somebody wrote, they mean the
same thing wherever the renderer puts them, and re-typing them across four
hundred reports is exactly the work this exists to avoid.

They are gathered here rather than in table.go because they are read on a second
pass, after the fields exist, and because none of them changes what the report
*says* — a wrong label is a cosmetic defect where a wrong column is a wrong
number.
*/

// present moves the labels, formats and captions onto the dataset's fields.
//
// The label belongs to the field rather than the block: cronos renders a table
// heading from the dataset, so "Customer" typed once in the column header band
// reaches every report that later binds to this dataset.
func (t *translation) present(ds *definition.Dataset) {
	labels := t.labels()
	formats := t.formats()
	for i, f := range ds.Fields {
		if label, ok := labels[f.Name]; ok && label != "" {
			ds.Fields[i].Label = label
		}
		if format, ok := formats[f.Name]; ok && format != "" {
			ds.Fields[i].Format = format
		}
		t.byName[f.Name] = ds.Fields[i]
	}
}

// formats reads each column's number format from the pattern Jasper drew it with.
//
// A Java DecimalFormat pattern says more than it looks: a currency symbol in it
// is the author stating that the column is money, which is the one formatting
// decision a report cannot get wrong without being wrong. The rest of the
// pattern — how many decimals, which grouping separator — is locale work the
// renderer does, so only the kind is carried.
func (t *translation) formats() map[string]string {
	out := map[string]string{}
	for _, p := range t.doc.Detail.fields() {
		name, ok := t.columnOfQuiet(p)
		if !ok {
			continue
		}
		if format := formatOf(p.field.Pattern); format != "" {
			out[name] = format
		}
	}
	return out
}

// formatOf reads a DecimalFormat pattern as one of cronos's formats.
func formatOf(pattern string) string {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return ""
	}
	switch {
	case strings.ContainsAny(p, "¤$€£¥₹"):
		return "currency"
	case strings.Contains(p, "%"):
		return "percent"
	case strings.ContainsAny(p, "#0"):
		// A plain numeric pattern. Worth saying, because it distinguishes a
		// column of quantities from one of identifiers that happen to be
		// numeric — which is the distinction the builder needs.
		return "number"
	}
	// A date pattern, or anything else. Dates format from the field's type.
	return ""
}

// captions reads the static text sitting beside each variable the report drew.
//
// "Total billed" is what somebody called the number, and it is a better tile
// label than either the column's own heading or the variable's identifier. The
// caption is found the way a reader finds it: the nearest text to the left, on
// the same line.
func (t *translation) captions() map[string]string {
	out := map[string]string{}
	for _, s := range t.allSections() {
		texts := s.texts()
		if len(texts) == 0 {
			continue
		}
		for _, p := range s.fields() {
			r, plain := plainRef(p.field.Expression)
			if !plain || r.sigil != 'V' {
				continue
			}
			if caption := leftOf(p, texts); caption != "" {
				out[r.name] = caption
			}
		}
	}
	return out
}

// leftOf returns the nearest static text ending before the field starts, on the
// same row.
func leftOf(p placedField, texts []placedText) string {
	best, bestX := "", -1
	for _, s := range texts {
		if !sameRow(s.y, p.y) {
			continue
		}
		// Ends at or before the value begins. A caption overlapping the number
		// it labels is not a caption.
		if end := s.x + s.text.Element.Width; end <= p.x && s.x > bestX {
			best, bestX = strings.TrimSpace(s.text.Text), s.x
		}
	}
	return collapse(strings.TrimSuffix(best, ":"))
}
