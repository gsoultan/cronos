package jrxml

import (
	"strings"
	"testing"
)

// TestTheShapesRealFilesTake covers the variations a four-hundred-file estate
// contains that a hand-written fixture does not.
//
// None of these is exotic. They are what you get from twelve years of Jaspersoft
// Studio on Windows, and each one breaks the table inference completely if it is
// not handled — an importer that reads none of the detail band produces a
// definition that loads, runs, and renders an empty report.
func TestTheShapesRealFilesTake(t *testing.T) {
	t.Run("a byte order mark", func(t *testing.T) {
		// Windows editors add one, and it sits before the XML declaration.
		res := importString(t, "\xef\xbb\xbf"+plainReport)
		assertColumns(t, res, "id", "total")
	})

	t.Run("CRLF line endings", func(t *testing.T) {
		res := importString(t, strings.ReplaceAll(plainReport, "\n", "\r\n"))
		assertColumns(t, res, "id", "total")
		// XML normalises CRLF to LF in character data, so the carriage returns
		// do not reach the YAML. Asserted rather than assumed: a query with \r
		// in it cannot be written as a literal block, and the definition would
		// come out as one long quoted line.
		if strings.Contains(res.Dataset.Query, "\r") {
			t.Errorf("carriage returns reached the query: %q", res.Dataset.Query)
		}
	})

	t.Run("class on the expression", func(t *testing.T) {
		// Jaspersoft Studio writes this on every text field.
		res := importString(t, strings.ReplaceAll(plainReport,
			"<textFieldExpression>", `<textFieldExpression class="java.lang.String">`))
		assertColumns(t, res, "id", "total")
	})

	t.Run("a DOCTYPE instead of a namespace", func(t *testing.T) {
		// JasperReports 1.x files carry a DTD reference and no namespace.
		res := importString(t, `<?xml version="1.0"?>
<!DOCTYPE jasperReport PUBLIC "-//JasperReports//DTD Report Design//EN"
 "http://jasperreports.sourceforge.net/dtds/jasperreport.dtd">
<jasperReport name="ancient">`+plainBody+`</jasperReport>`)
		assertColumns(t, res, "id", "total")
	})

	t.Run("a column that calls toString on its field", func(t *testing.T) {
		// Found in JasperReports' own samples. `$F{OrderId}.toString()` is the
		// same column as `$F{OrderId}` — the call changed how Java rendered it,
		// not which field it is — and dropping it dropped the column, which is
		// a worse import than a number formatted by a different rule.
		res := importString(t, wrap(`
			<queryString><![CDATA[SELECT order_id, total FROM t]]></queryString>
			<field name="order_id" class="java.lang.Integer"/>
			<field name="total" class="java.math.BigDecimal"/>
			<detail><band height="20">
				<textField><reportElement x="0" y="0" width="100" height="20"/>
					<textFieldExpression><![CDATA[$F{order_id}.toString()]]></textFieldExpression></textField>
				<textField><reportElement x="110" y="0" width="100" height="20"/>
					<textFieldExpression><![CDATA[$F{total}]]></textFieldExpression></textField>
			</band></detail>`))
		assertColumns(t, res, "order_id", "total")
		if !hasFindingText(res, "toString") {
			t.Errorf("the column was recovered without saying the coercion was dropped:\n%s", render(res))
		}
		// Still not a licence to import arithmetic.
		other := importString(t, wrap(`
			<queryString><![CDATA[SELECT a, b FROM t]]></queryString>
			<field name="a" class="java.lang.String"/>
			<field name="b" class="java.lang.String"/>
			<detail><band height="20">
				<textField><reportElement x="0" y="0" width="100" height="20"/>
					<textFieldExpression><![CDATA[$F{a} + $F{b}.toString()]]></textFieldExpression></textField>
				<textField><reportElement x="110" y="0" width="100" height="20"/>
					<textFieldExpression><![CDATA[$F{b}]]></textFieldExpression></textField>
			</band></detail>`))
		assertColumns(t, other, "b")
	})

	t.Run("the JasperReports 7 query spelling", func(t *testing.T) {
		// JasperReports 7 renamed <queryString> to <query> and changed nothing
		// else about it. Found by running the importer over the library's own
		// samples on master, where every file came back with nothing at all:
		// the layout is a syntax this does not read, and the query was going
		// unread too, so a JR7 file yielded no dataset either. It yields one now.
		res := importString(t, wrap(`
			<query language="sql"><![CDATA[SELECT id, total FROM t WHERE d >= $P{From}]]></query>
			<field name="id" class="java.lang.String"/>
			<field name="total" class="java.math.BigDecimal"/>
			<parameter name="From" class="java.util.Date"/>
			<detail><band height="20">
				<textField><reportElement x="0" y="0" width="100" height="20"/>
					<textFieldExpression><![CDATA[$F{id}]]></textFieldExpression></textField>
			</band></detail>`))
		if !res.HasDataset() {
			t.Fatalf("a <query> element yielded no dataset:\n%s", render(res))
		}
		if !strings.Contains(res.Dataset.Query, "{{ .params.from }}") {
			t.Errorf("the JR7 query did not bind its parameter:\n%s", res.Dataset.Query)
		}
		if len(res.Dataset.Fields) != 2 {
			t.Errorf("got %d fields, want 2", len(res.Dataset.Fields))
		}
	})

	t.Run("both spellings do not fight", func(t *testing.T) {
		// A file carrying both is not something Jasper writes, but the reader
		// has to pick one rather than concatenate or blank them.
		res := importString(t, wrap(`
			<queryString><![CDATA[SELECT id FROM classic]]></queryString>
			<query language="sql"><![CDATA[SELECT id FROM modern]]></query>
			<field name="id" class="java.lang.String"/>
			<detail><band height="20">
				<textField><reportElement x="0" y="0" width="100" height="20"/>
					<textFieldExpression><![CDATA[$F{id}]]></textFieldExpression></textField>
			</band></detail>`))
		if !strings.Contains(res.Dataset.Query, "classic") {
			t.Errorf("the classic spelling did not win:\n%s", res.Dataset.Query)
		}
	})

	t.Run("the JasperReports 7 band syntax", func(t *testing.T) {
		// `<element kind="textField">` with the geometry as attributes and an
		// `<expression>` child, where 6.x wrote `<textField>` around a
		// `<reportElement>`. Taken from the shape of JasperReports' own master
		// samples, where this importer used to read nothing at all.
		res := importString(t, wrap(`
			<query language="sql"><![CDATA[SELECT id, name, total FROM t]]></query>
			<field name="id" class="java.lang.Integer"/>
			<field name="name" class="java.lang.String"/>
			<field name="total" class="java.math.BigDecimal"/>
			<columnHeader><band height="15">
				<element kind="staticText" x="0" y="0" width="50" height="15"><text><![CDATA[Id]]></text></element>
				<element kind="staticText" x="50" y="0" width="200" height="15"><text><![CDATA[Name]]></text></element>
			</band></columnHeader>
			<detail><band height="15">
				<element kind="textField" x="0" y="0" width="50" height="15"><expression><![CDATA[$F{id}]]></expression></element>
				<element kind="textField" x="50" y="0" width="200" height="15"><expression><![CDATA[$F{name}]]></expression></element>
				<element kind="line" x="0" y="14" width="250" height="1"/>
			</band></detail>`))

		assertColumns(t, res, "id", "name")
		// The header band is read in the same dialect, so labels still land.
		if f, _ := res.Dataset.Field("name"); f.Label != "Name" {
			t.Errorf("column label = %q, want Name", f.Label)
		}
		if res.Blocked() {
			t.Errorf("a readable JR7 layout was reported as needing a person:\n%s", render(res))
		}
	})

	t.Run("a JasperReports 7 kind that is not read still reports", func(t *testing.T) {
		// Every element in this dialect is named `element`, so the census sees
		// one name and learns nothing. The kind attribute is where the
		// information is, and a construct that vanished quietly would be the
		// only one in the importer that did.
		res := importString(t, wrap(`
			<query language="sql"><![CDATA[SELECT id FROM t]]></query>
			<field name="id" class="java.lang.String"/>
			<detail><band height="20">
				<element kind="textField" x="0" y="0" width="100" height="20"><expression><![CDATA[$F{id}]]></expression></element>
				<element kind="crosstab" x="0" y="30" width="300" height="200"/>
				<element kind="somethingNew" x="0" y="240" width="300" height="20"/>
			</band></detail>`))

		assertColumns(t, res, "id")
		if !hasFinding(res, "crosstab") {
			t.Errorf("a JR7 crosstab went unreported:\n%s", render(res))
		}
		if !hasFinding(res, "somethingNew") {
			t.Errorf("an unrecognised JR7 kind went unreported:\n%s", render(res))
		}
	})
}

func assertColumns(t *testing.T, res Result, want ...string) {
	t.Helper()
	pdf, ok := res.Report.Output("pdf")
	if !ok {
		t.Fatalf("no pdf output; findings:\n%s", render(res))
	}
	table := blockOfKind(t, pdf.Layout, "table")
	if strings.Join(table.Columns, ",") != strings.Join(want, ",") {
		t.Errorf("columns = %v, want %v", table.Columns, want)
	}
}

const plainBody = `
	<queryString><![CDATA[SELECT id, total
FROM t]]></queryString>
	<field name="id" class="java.lang.String"/>
	<field name="total" class="java.math.BigDecimal"/>
	<detail><band height="20">
		<textField><reportElement x="0" y="0" width="100" height="20"/>
			<textFieldExpression><![CDATA[$F{id}]]></textFieldExpression></textField>
		<textField><reportElement x="110" y="0" width="100" height="20"/>
			<textFieldExpression><![CDATA[$F{total}]]></textFieldExpression></textField>
	</band></detail>`

var plainReport = wrap(plainBody)
