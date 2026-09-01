package jrxml

import (
	"strings"
	"testing"
)

// TestFindingsMergeAndSort pins the shape of the work list, which is the half of
// this feature a migrating team actually reads.
func TestFindingsMergeAndSort(t *testing.T) {
	var f findings
	f.add(Note, "appearance", "fonts are not carried")
	f.add(Review, "subreport", "a subreport is missing")
	f.add(Note, "appearance", "fonts are not carried")
	f.add(Blocked, "queryString", "not SQL")
	f.addN(Note, "appearance", "fonts are not carried", 9)

	got := f.sorted()
	if len(got) != 3 {
		t.Fatalf("got %d findings, want 3 after merging: %+v", len(got), got)
	}
	// Worst first: an estate is triaged in that order.
	if got[0].Severity != Blocked || got[1].Severity != Review || got[2].Severity != Note {
		t.Errorf("not sorted worst-first: %+v", got)
	}
	if got[2].Count != 11 {
		t.Errorf("repeats counted %d, want 11", got[2].Count)
	}
	if line := got[2].String(); !strings.HasPrefix(line, "note: appearance —") || !strings.HasSuffix(line, "(11)") {
		t.Errorf("finding line = %q", line)
	}
	if line := got[0].String(); line != "blocked: queryString — not SQL" {
		t.Errorf("a finding seen once should not print a count: %q", line)
	}
}

// TestWorstOccurrenceSetsTheGrade: the same construct can be cosmetic in one
// place and load-bearing in another, and the list has to show the worse one.
func TestWorstOccurrenceSetsTheGrade(t *testing.T) {
	var f findings
	f.add(Note, "chart", "the same thing")
	f.add(Review, "chart", "the same thing")
	got := f.sorted()
	if len(got) != 1 || got[0].Severity != Review || got[0].Count != 2 {
		t.Errorf("got %+v, want one Review with count 2", got)
	}
}

// TestNeedsCounts backs the summary line an operator reads.
func TestNeedsCounts(t *testing.T) {
	r := Result{Findings: []Finding{
		{Severity: Blocked}, {Severity: Review}, {Severity: Review}, {Severity: Note},
	}}
	if got := r.Needs(Review); got != 3 {
		t.Errorf("Needs(Review) = %d, want 3 — blocked counts as needing review too", got)
	}
	if got := r.Needs(Blocked); got != 1 {
		t.Errorf("Needs(Blocked) = %d, want 1", got)
	}
	if !r.Blocked() {
		t.Error("Blocked() missed a blocking finding")
	}
}

func TestSeverityStrings(t *testing.T) {
	for s, want := range map[Severity]string{Blocked: "blocked", Review: "review", Note: "note"} {
		if got := s.String(); got != want {
			t.Errorf("Severity(%d) = %q, want %q", s, got, want)
		}
	}
}

// TestFooterFallsBackToLastPage covers a report that numbers its pages only in
// the last-page footer, and a footer that draws something a one-line cronos
// footer cannot hold.
func TestFooterFallsBackToLastPage(t *testing.T) {
	t.Run("last page footer is read when the page footer is empty", func(t *testing.T) {
		res := importString(t, wrap(`
			<queryString><![CDATA[SELECT id FROM t]]></queryString>
			<field name="id" class="java.lang.String"/>
			<pageFooter><band height="20"/></pageFooter>
			<lastPageFooter><band height="20">
				<textField><reportElement x="0" y="0" width="80" height="20"/>
					<textFieldExpression><![CDATA["Page " + $V{PAGE_NUMBER}]]></textFieldExpression></textField>
			</band></lastPageFooter>
			<detail><band height="20">
				<textField><reportElement x="0" y="0" width="100" height="20"/>
					<textFieldExpression><![CDATA[$F{id}]]></textFieldExpression></textField>
			</band></detail>`))
		pdf, _ := res.Report.Output("pdf")
		if pdf.Footer.Text != "Page {{ .page }}" {
			t.Errorf("footer = %q, want the page number from the last-page footer", pdf.Footer.Text)
		}
	})

	t.Run("a footer that is not a page number is reported", func(t *testing.T) {
		res := importString(t, wrap(`
			<queryString><![CDATA[SELECT id FROM t]]></queryString>
			<field name="id" class="java.lang.String"/>
			<pageFooter><band height="20">
				<staticText><reportElement x="0" y="0" width="80" height="20"/>
					<text><![CDATA[Confidential]]></text></staticText>
				<staticText><reportElement x="100" y="0" width="80" height="20"/>
					<text><![CDATA[Acme Ltd]]></text></staticText>
			</band></pageFooter>
			<detail><band height="20">
				<textField><reportElement x="0" y="0" width="100" height="20"/>
					<textFieldExpression><![CDATA[$F{id}]]></textFieldExpression></textField>
			</band></detail>`))
		pdf, _ := res.Report.Output("pdf")
		if pdf.Footer.Text != "" {
			t.Errorf("footer = %q, want none — two lines do not fit one", pdf.Footer.Text)
		}
		if !hasFinding(res, "pageFooter") {
			t.Errorf("the dropped footer was not reported:\n%s", render(res))
		}
	})

	t.Run("a single line footer is carried", func(t *testing.T) {
		res := importString(t, wrap(`
			<queryString><![CDATA[SELECT id FROM t]]></queryString>
			<field name="id" class="java.lang.String"/>
			<pageFooter><band height="20">
				<staticText><reportElement x="0" y="0" width="200" height="20"/>
					<text><![CDATA[Confidential — Acme Ltd]]></text></staticText>
			</band></pageFooter>
			<detail><band height="20">
				<textField><reportElement x="0" y="0" width="100" height="20"/>
					<textFieldExpression><![CDATA[$F{id}]]></textFieldExpression></textField>
			</band></detail>`))
		pdf, _ := res.Report.Output("pdf")
		if pdf.Footer.Text != "Confidential — Acme Ltd" {
			t.Errorf("footer = %q", pdf.Footer.Text)
		}
	})
}
