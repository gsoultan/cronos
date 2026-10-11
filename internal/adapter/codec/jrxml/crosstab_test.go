package jrxml

import (
	"strings"
	"testing"
)

// The legacy file's crosstab — customers down the side, dates across, the
// amount summed — is a pivot now, on the interactive output beside the detail
// table it summarised.
func TestACrosstabImportsAsAPivot(t *testing.T) {
	res := importFixture(t, "legacy.jrxml")

	out, ok := res.Report.Output("interactive")
	if !ok {
		t.Fatal("no interactive output")
	}
	p := blockOfKind(t, out.Layout, "pivot")
	if p.Pivot.Rows.Field != "kundename" || p.Pivot.Columns.Field != "datum" ||
		p.Value.Field != "betrag" || p.Value.Aggregate != "sum" {
		t.Errorf("pivot = %+v of %+v", p.Pivot, p.Value)
	}
	// Dates across the top, unbucketed, is a column a day; the fix is a word.
	if !hasFindingText(res, "set grain: month on the pivot") {
		t.Errorf("an unbucketed date crossed without a note:\n%s", render(res))
	}
	// The PDF prints one table, and it is the detail's.
	pdf, _ := res.Report.Output("pdf")
	for _, b := range pdf.Layout {
		if b.Kind == "pivot" {
			t.Error("a pivot was laid out on a document that will never print it")
		}
	}
	if !hasFindingText(res, "interactive output only") {
		t.Errorf("the pivot left off the PDF is not reported:\n%s", render(res))
	}
}

const crosstabOnly = `<?xml version="1.0"?>
<jasperReport name="Sales matrix">
  <queryString><![CDATA[SELECT region, product, amount FROM sales]]></queryString>
  <field name="region" class="java.lang.String"/>
  <field name="product" class="java.lang.String"/>
  <field name="amount" class="java.math.BigDecimal"/>
  <summary><band height="200">
    <crosstab>
      <reportElement x="0" y="0" width="540" height="180"/>
      %s
      <rowGroup name="r"><bucket><bucketExpression><![CDATA[$F{region}]]></bucketExpression></bucket></rowGroup>
      <columnGroup name="c"><bucket><bucketExpression><![CDATA[%s]]></bucketExpression></bucket></columnGroup>
      <measure name="m" calculation="Average"><measureExpression><![CDATA[$F{amount}]]></measureExpression></measure>
    </crosstab>
  </band></summary>
</jasperReport>`

func crosstabFile(dataset, column string) string {
	return strings.Replace(strings.Replace(crosstabOnly, "%s", dataset, 1), "%s", column, 1)
}

// A report that is nothing but a crosstab prints it: the pivot is the table.
func TestACrosstabAloneIsThePrintedTable(t *testing.T) {
	res := importString(t, crosstabFile("", "$F{product}"))
	pdf, ok := res.Report.Output("pdf")
	if !ok {
		t.Fatalf("no pdf output:\n%s", render(res))
	}
	p := blockOfKind(t, pdf.Layout, "pivot")
	if p.Value.Aggregate != "avg" || p.Title != "Amount by Region and Product" {
		t.Errorf("pivot %q folds by %q", p.Title, p.Value.Aggregate)
	}
}

// What a pivot cannot read is said rather than half-imported.
func TestACrosstabThatCannotBeAPivotIsReported(t *testing.T) {
	for name, file := range map[string]string{
		"a second query": crosstabFile(
			`<crosstabDataset><dataset><datasetRun subDataset="other"/></dataset></crosstabDataset>`, "$F{product}"),
		"java": crosstabFile("", `$F{product}.toUpperCase()`),
	} {
		t.Run(name, func(t *testing.T) {
			res := importString(t, file)
			if out, ok := res.Report.Output("interactive"); ok {
				for _, b := range out.Layout {
					if b.Kind == "pivot" {
						t.Errorf("imported %+v", b)
					}
				}
			}
			if !hasFinding(res, "crosstab") {
				t.Errorf("nothing said:\n%s", render(res))
			}
		})
	}
}
