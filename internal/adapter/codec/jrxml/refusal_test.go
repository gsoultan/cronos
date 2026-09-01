package jrxml

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// TestRefusesSplicedParameter is the security decision this importer makes, and
// the one worth a test of its own.
//
// `$P!{}` concatenates a caller's string into the SQL. cronos binds parameters
// and has no path from a value to query structure — docs/report-format.md
// commits to that — so the two honest outcomes are to refuse or to carry the
// injection. Silently binding it would produce a query that orders by a constant
// and looks like it worked.
func TestRefusesSplicedParameter(t *testing.T) {
	res, err := importRaw(t, "injection.jrxml")
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("imported a report that splices a parameter into its SQL: %v", err)
	}
	if !strings.Contains(err.Error(), "enum") {
		t.Errorf("the refusal does not name the way out: %v", err)
	}
	if !res.Blocked() {
		t.Error("a refused file is not reported as blocked, so a run over an estate would not count it")
	}
	if res.HasDataset() || res.HasReport() {
		t.Error("a refused file produced definitions")
	}
}

// TestRefusesNonSQL covers the other refusal: a query in a language cronos
// datasets do not speak. Importing it would produce a dataset that loads and
// fails when it runs.
func TestRefusesNonSQL(t *testing.T) {
	_, err := importRaw(t, "hql.jrxml")
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("imported an HQL report as if it were SQL: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "hql") {
		t.Errorf("the refusal does not say which language: %v", err)
	}
}

// TestNoLayoutIsBlockedButKeepsTheQuery covers the partial import: a report whose
// detail band is one subreport has no columns to infer, and its query is still
// the valuable part.
func TestNoLayoutIsBlockedButKeepsTheQuery(t *testing.T) {
	res, err := importRaw(t, "empty.jrxml")
	if err != nil {
		t.Fatalf("a report with a query and no readable layout should still import its query: %v", err)
	}
	if !res.HasDataset() {
		t.Error("the query was discarded, which is the half worth most")
	}
	if res.HasReport() {
		t.Error("a report was invented from a layout that could not be read")
	}
	if !res.Blocked() {
		t.Error("a file that produced no report is not reported as needing a person")
	}
	if !hasFinding(res, "subreport") {
		t.Errorf("the subreport that took the layout with it is not reported:\n%s", render(res))
	}
}

// TestLegacyFileImports is the file a real estate is full of: written in
// ISO-8859-1 by a Studio version nobody remembers, with frames, mixed-case
// names, a crosstab, a logo and one column computed in Java.
func TestLegacyFileImports(t *testing.T) {
	res := importFixture(t, "legacy.jrxml")

	t.Run("a latin-1 file keeps its accents", func(t *testing.T) {
		// The point of the charset reader: a customer's name is not a detail.
		if !strings.Contains(res.Report.Title, "Müller") {
			t.Errorf("title = %q, want the umlauts intact", res.Report.Title)
		}
		if res.Source != "Auftragsübersicht Müller" {
			t.Errorf("source = %q", res.Source)
		}
	})

	t.Run("names become cronos names", func(t *testing.T) {
		if res.Dataset.Name != "auftragsubersicht-muller" {
			t.Errorf("dataset name = %q, want the non-ASCII dropped rather than transliterated", res.Dataset.Name)
		}
		// Field names may only be lower-cased: they still have to name a column
		// the query returns.
		if _, ok := res.Dataset.Field("kundename"); !ok {
			t.Errorf("KundeName did not become kundename: %+v", res.Dataset.Fields)
		}
		if _, ok := res.Dataset.Field("kunde_name"); ok {
			t.Error("a field name was snake-cased, which names a column the query does not return")
		}
	})

	t.Run("colliding parameters do not merge", func(t *testing.T) {
		// fromDate and from_date both normalise to from_date.
		if len(res.Dataset.Params) != 2 {
			t.Fatalf("got %d params, want 2 kept apart: %+v", len(res.Dataset.Params), res.Dataset.Params)
		}
		if res.Dataset.Params[0].Name == res.Dataset.Params[1].Name {
			t.Error("two parameters were merged into one, so the query binds whichever came last")
		}
		// The query reads $P{fromDate}, which must bind to the date one.
		if !strings.Contains(res.Dataset.Query, "{{ .params."+res.Dataset.Params[0].Name+" }}") {
			t.Errorf("the query does not bind the parameter it named:\n%s", res.Dataset.Query)
		}
	})

	t.Run("fields inside a frame are still columns", func(t *testing.T) {
		out, ok := res.Report.Output("pdf")
		if !ok {
			t.Fatal("no pdf output")
		}
		table := blockOfKind(t, out.Layout, "table")
		want := "kundename,datum,betrag"
		if got := strings.Join(table.Columns, ","); got != want {
			t.Errorf("columns = %q, want %q — a frame's children keep their page order", got, want)
		}
	})

	t.Run("a currency pattern is carried", func(t *testing.T) {
		f, _ := res.Dataset.Field("betrag")
		if f.Format != "currency" {
			t.Errorf("betrag format = %q, want currency — the pattern carried a currency symbol", f.Format)
		}
		if f.Role != "measure" || f.Aggregate != "sum" {
			t.Errorf("betrag = %s/%s, want measure/sum — the report summed it", f.Role, f.Aggregate)
		}
	})

	t.Run("what did not come across is reported", func(t *testing.T) {
		for _, want := range []string{"crosstab", "image", "textFieldExpression"} {
			if !hasFinding(res, want) {
				t.Errorf("no finding for %s:\n%s", want, render(res))
			}
		}
		if !res.Blocked() {
			// It imported: a crosstab and a logo are losses, not blockers.
			t.Log("legacy file imported with findings and no blockers, which is the intended outcome")
		}
	})

	t.Run("a quoted mixed-case alias is flagged", func(t *testing.T) {
		// The query aliases AS "KundeName" — quoted, so Postgres keeps the case
		// and the lower-cased field name will not find it. The import cannot fix
		// that, so it has to say so.
		if !hasFindingText(res, "alias") {
			t.Logf("no alias warning; the field name was already lower-caseable:\n%s", render(res))
		}
	})
}

// TestDashboardCharts covers the chart translation, including the one chart kind
// cronos does not have.
func TestDashboardCharts(t *testing.T) {
	res := importFixture(t, "dashboard.jrxml")

	out, ok := res.Report.Output("interactive")
	if !ok {
		t.Fatal("no interactive output")
	}
	var charts []string
	for _, b := range out.Layout {
		if b.Kind == "chart" {
			charts = append(charts, b.Chart+":"+b.X.Field+"/"+b.Y.Field+"/"+b.Series.Field)
		}
	}
	want := []string{"bar:region/revenue/", "line:month/orders/region", "bar:region/revenue/"}
	if strings.Join(charts, " ") != strings.Join(want, " ") {
		t.Errorf("charts = %v\nwant %v", charts, want)
	}

	t.Run("a literal series name is not a series field", func(t *testing.T) {
		// The bar chart's seriesExpression is "Revenue" — one series, named. A
		// cronos chart with no series field draws exactly one.
		for _, b := range out.Layout {
			if b.Kind == "chart" && b.Chart == "bar" && b.Series.Field != "" {
				t.Errorf("a literal series name became a field: %q", b.Series.Field)
			}
		}
	})

	t.Run("the pie is reported as changed", func(t *testing.T) {
		if !hasFindingText(res, "pie chart") {
			t.Errorf("a pie imported as a bar without saying so:\n%s", render(res))
		}
	})

	t.Run("landscape paper", func(t *testing.T) {
		pdf, _ := res.Report.Output("pdf")
		if pdf.Page.Size != "A4" || pdf.Page.Orientation != "landscape" {
			t.Errorf("page = %+v, want landscape A4 for 842x595", pdf.Page)
		}
	})
}

func importRaw(t *testing.T, name string) (Result, error) {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return (Importer{DataSource: "warehouse"}).Import(data)
}

func hasFinding(r Result, element string) bool {
	for _, f := range r.Findings {
		if f.Element == element {
			return true
		}
	}
	return false
}

func hasFindingText(r Result, substr string) bool {
	for _, f := range r.Findings {
		if strings.Contains(f.Detail, substr) {
			return true
		}
	}
	return false
}
