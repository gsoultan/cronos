package jrxml

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestDeepNestingIsRefusedNotFatal covers the shape that turns a recursive
// reader into a crash.
//
// contents.textFieldsAt recurses once per frame, and a file with a hundred
// thousand nested frames would recurse a hundred thousand deep. Go's XML
// decoder caps nesting first, so this arrives as an error rather than as a
// stack that grows until the process dies — which matters because the process
// is importing four hundred files and the other three hundred and ninety-nine
// should still get imported.
func TestDeepNestingIsRefusedNotFatal(t *testing.T) {
	deep := func(n int) []byte {
		return []byte(`<jasperReport name="deep"><queryString>SELECT 1</queryString>` +
			`<field name="a" class="java.lang.String"/><detail><band height="20">` +
			strings.Repeat(`<frame><reportElement x="1" y="1" width="9" height="9"/>`, n) +
			`<textField><reportElement x="0" y="0" width="10" height="20"/>` +
			`<textFieldExpression><![CDATA[$F{a}]]></textFieldExpression></textField>` +
			strings.Repeat(`</frame>`, n) + `</band></detail></jasperReport>`)
	}

	// Deep, but within what the decoder allows: still a working import.
	res, err := (Importer{DataSource: "warehouse"}).Import(deep(2000))
	if err != nil {
		t.Fatalf("a deeply framed but legal report was refused: %v", err)
	}
	if !res.HasReport() {
		t.Error("a text field two thousand frames down was not found")
	}

	// Past the decoder's limit: an error naming the file's problem, and no panic.
	if _, err := (Importer{DataSource: "warehouse"}).Import(deep(20000)); !errors.Is(err, ErrParse) {
		t.Errorf("nesting past the decoder's limit gave %v, want an ErrParse", err)
	}
}

// TestWideReportImportsEveryColumn covers the other direction: a report with
// far more columns than anyone would draw on a page.
//
// Wide reports are real — an export-shaped .jrxml with sixty columns is common —
// and the inference pairs every detail field against every header, so this is
// where the cost is. Asserted on the result rather than on a clock, because a
// timing assertion is a test that fails on somebody else's busy CI.
func TestWideReportImportsEveryColumn(t *testing.T) {
	const n = 500
	var fields, headers, detail strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&fields, `<field name="c%d" class="java.lang.String"/>`, i)
		fmt.Fprintf(&headers, `<staticText><reportElement x="%d" y="0" width="10" height="20"/>`+
			`<text><![CDATA[H%d]]></text></staticText>`, i*10, i)
		fmt.Fprintf(&detail, `<textField><reportElement x="%d" y="0" width="10" height="20"/>`+
			`<textFieldExpression><![CDATA[$F{c%d}]]></textFieldExpression></textField>`, i*10, i)
	}
	doc := `<jasperReport name="wide"><queryString>SELECT 1</queryString>` + fields.String() +
		`<columnHeader><band height="20">` + headers.String() + `</band></columnHeader>` +
		`<detail><band height="20">` + detail.String() + `</band></detail></jasperReport>`

	res := importString(t, doc)
	pdf, ok := res.Report.Output("pdf")
	if !ok {
		t.Fatal("no pdf output")
	}
	table := blockOfKind(t, pdf.Layout, "table")
	if len(table.Columns) != n {
		t.Errorf("imported %d of %d columns", len(table.Columns), n)
	}
	// Each column takes the heading sitting over it, not its neighbour's.
	for _, want := range []struct{ field, label string }{
		{"c0", "H0"}, {"c250", "H250"}, {"c499", "H499"},
	} {
		f, _ := res.Dataset.Field(want.field)
		if f.Label != want.label {
			t.Errorf("field %s label = %q, want %q", want.field, f.Label, want.label)
		}
	}
}
