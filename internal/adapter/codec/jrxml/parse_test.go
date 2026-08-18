package jrxml

import (
	"errors"
	"strings"
	"testing"
)

// TestParseRefusesWhatIsNotAReport covers the files that share a directory with
// the reports: compiled .jasper, print output, data adapters, and the file
// somebody truncated.
func TestParseRefusesWhatIsNotAReport(t *testing.T) {
	for name, body := range map[string]string{
		"not xml at all":   "this is not xml",
		"a different root": `<?xml version="1.0"?><jasperPrint name="x"/>`,
		"truncated":        `<?xml version="1.0"?><jasperReport name="x"><queryString>`,
		"no name":          `<?xml version="1.0"?><jasperReport><queryString/></jasperReport>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parse([]byte(body)); !errors.Is(err, ErrParse) {
				t.Errorf("parsed %s: %v", name, err)
			}
		})
	}
}

// TestUnknownCharsetSaysSo is the alternative to mangling a customer's name: an
// encoding this cannot decode is an error naming the fix, not replacement
// characters in every statement.
func TestUnknownCharsetSaysSo(t *testing.T) {
	_, err := parse([]byte(`<?xml version="1.0" encoding="Shift_JIS"?><jasperReport name="x"/>`))
	if !errors.Is(err, ErrParse) {
		t.Fatalf("accepted an encoding it cannot read: %v", err)
	}
	if !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("the error does not name the fix: %v", err)
	}
}

// TestCensusReportsWhatNobodyClassified is the safety net, and the reason this
// importer can claim nothing is dropped silently.
//
// JasperReports has some two hundred elements across six versions plus a
// component namespace anyone can extend. An importer that reported only the
// constructs its author remembered would be silent about the rest.
func TestCensusReportsWhatNobodyClassified(t *testing.T) {
	res := importString(t, wrap(`
		<queryString><![CDATA[SELECT id FROM t]]></queryString>
		<field name="id" class="java.lang.String"/>
		<detail><band height="20">
			<somethingFromTheFuture x="1"><innerFutureThing/></somethingFromTheFuture>
			<textField><reportElement x="0" y="0" width="100" height="20"/>
				<textFieldExpression><![CDATA[$F{id}]]></textFieldExpression></textField>
		</band></detail>`))

	if !hasFinding(res, "somethingFromTheFuture") {
		t.Errorf("an unrecognised element was dropped silently:\n%s", render(res))
	}
	if !hasFinding(res, "innerFutureThing") {
		t.Errorf("an unrecognised nested element was dropped silently:\n%s", render(res))
	}
}

// TestOnlyTheOutermostGroup covers the limit of a cronos table: one groupBy.
func TestOnlyTheOutermostGroup(t *testing.T) {
	res := importString(t, wrap(`
		<queryString><![CDATA[SELECT region, city, total FROM sales]]></queryString>
		<field name="region" class="java.lang.String"/>
		<field name="city" class="java.lang.String"/>
		<field name="total" class="java.math.BigDecimal"/>
		<group name="ByRegion"><groupExpression><![CDATA[$F{region}]]></groupExpression></group>
		<group name="ByCity"><groupExpression><![CDATA[$F{city}]]></groupExpression></group>
		<detail><band height="20">
			<textField><reportElement x="0" y="0" width="100" height="20"/>
				<textFieldExpression><![CDATA[$F{total}]]></textFieldExpression></textField>
		</band></detail>`))

	pdf, ok := res.Report.Output("pdf")
	if !ok {
		t.Fatal("no pdf output")
	}
	table := blockOfKind(t, pdf.Layout, "table")
	if table.GroupBy != "region" {
		t.Errorf("groupBy = %q, want the outermost group", table.GroupBy)
	}
	if !hasFindingText(res, "ByCity") {
		t.Errorf("the nested group was dropped without saying so:\n%s", render(res))
	}
}

// TestUnsupportedCalculationIsNotApproximated covers the folds cronos does not
// have. A standard deviation silently becoming a sum is a subtotal nobody could
// account for.
func TestUnsupportedCalculationIsNotApproximated(t *testing.T) {
	res := importString(t, wrap(`
		<queryString><![CDATA[SELECT region, total FROM sales]]></queryString>
		<field name="region" class="java.lang.String"/>
		<field name="total" class="java.math.BigDecimal"/>
		<variable name="SD" class="java.lang.Double" resetType="Group" resetGroup="ByRegion"
		          calculation="StandardDeviation">
			<variableExpression><![CDATA[$F{total}]]></variableExpression>
		</variable>
		<group name="ByRegion"><groupExpression><![CDATA[$F{region}]]></groupExpression></group>
		<detail><band height="20">
			<textField><reportElement x="0" y="0" width="100" height="20"/>
				<textFieldExpression><![CDATA[$F{total}]]></textFieldExpression></textField>
		</band></detail>`))

	pdf, _ := res.Report.Output("pdf")
	table := blockOfKind(t, pdf.Layout, "table")
	if len(table.Subtotals) != 0 {
		t.Errorf("subtotals = %v, want none — a standard deviation is not a cronos aggregate", table.Subtotals)
	}
	if !hasFindingText(res, "StandardDeviation") {
		t.Errorf("the dropped subtotal was not reported:\n%s", render(res))
	}
}

// TestComputedParameterIsReported covers a parameter the report used to work out
// for itself. A caller now has to supply it, which is a change to the report's
// interface and not a cosmetic one.
func TestComputedParameterIsReported(t *testing.T) {
	res := importString(t, wrap(`
		<parameter name="PERIOD_START" class="java.util.Date" isForPrompting="false"/>
		<queryString><![CDATA[SELECT id FROM t WHERE d >= $P{PERIOD_START}]]></queryString>
		<field name="id" class="java.lang.String"/>
		<detail><band height="20">
			<textField><reportElement x="0" y="0" width="100" height="20"/>
				<textFieldExpression><![CDATA[$F{id}]]></textFieldExpression></textField>
		</band></detail>`))

	p, ok := res.Dataset.Param("period_start")
	if !ok {
		t.Fatal("a parameter the query reads was not declared")
	}
	if p.Required {
		t.Error("a parameter the report computed was imported as required, which no caller was passing")
	}
	if !hasFindingText(res, "computed by the report") {
		t.Errorf("the change to the report's interface was not reported:\n%s", render(res))
	}
}

// TestReservedParameterInQueryIsRefused covers a query reading Jasper's own
// plumbing. There is no cronos equivalent, so the query would not compile.
func TestReservedParameterInQueryIsRefused(t *testing.T) {
	_, err := (Importer{DataSource: "warehouse"}).Import([]byte(wrap(`
		<parameter name="REPORT_MAX_COUNT" class="java.lang.Integer"/>
		<queryString><![CDATA[SELECT id FROM t LIMIT $P{REPORT_MAX_COUNT}]]></queryString>
		<field name="id" class="java.lang.String"/>`)))
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("imported a query reading a JasperReports built-in: %v", err)
	}
}

// TestDataSourceIsRequired: a .jrxml names no database, so guessing one would
// produce definitions that load and cannot run.
func TestDataSourceIsRequired(t *testing.T) {
	_, err := (Importer{}).Import([]byte(wrap(`<queryString><![CDATA[SELECT 1]]></queryString>`)))
	if err == nil || !strings.Contains(err.Error(), "DataSource") {
		t.Errorf("imported without a datasource: %v", err)
	}
}

// wrap puts a body inside a minimal jasperReport root.
func wrap(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport xmlns="http://jasperreports.sourceforge.net/jasperreports" name="fixture">` +
		body + `</jasperReport>`
}

func importString(t *testing.T, body string) Result {
	t.Helper()
	res, err := (Importer{DataSource: "warehouse"}).Import([]byte(body))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	return res
}
