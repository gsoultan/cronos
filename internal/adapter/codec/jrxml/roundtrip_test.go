package jrxml

import (
	"os"
	"strings"
	"testing"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

// TestStatementImports walks the whole feature on a report shaped like the ones
// a migration actually holds: a grouped, page-broken, subtotalled statement.
//
// The assertions are about meaning rather than bytes. What matters is that the
// query still asks the same question with bound parameters, that the columns are
// the ones the detail band drew in the order it drew them, and that what came out
// is a definition the engine will load — which is asserted by loading it.
func TestStatementImports(t *testing.T) {
	res := importFixture(t, "statement.jrxml")

	t.Run("query binds its parameters", func(t *testing.T) {
		q := res.Dataset.Query
		for _, want := range []string{
			"{{ .params.from_date }}", "{{ .params.to_date }}", "{{ .params.status_list }}",
		} {
			if !strings.Contains(q, want) {
				t.Errorf("query is missing %s:\n%s", want, q)
			}
		}
		if strings.Contains(q, "$P{") {
			t.Errorf("query still holds a Jasper parameter:\n%s", q)
		}
	})

	t.Run("parameters keep their types", func(t *testing.T) {
		want := map[string]struct {
			kind     string
			multiple bool
			def      any
		}{
			"from_date":   {"date", false, "today"},
			"to_date":     {"date", false, nil},
			"status_list": {"string", true, nil},
		}
		if len(res.Dataset.Params) != len(want) {
			t.Fatalf("got %d params, want %d: %+v", len(res.Dataset.Params), len(want), res.Dataset.Params)
		}
		for _, p := range res.Dataset.Params {
			w, known := want[p.Name]
			if !known {
				t.Errorf("unexpected param %q", p.Name)
				continue
			}
			if string(p.Type) != w.kind || p.Multiple != w.multiple || p.Default != w.def {
				t.Errorf("param %q = {%s multiple:%v default:%v}, want {%s multiple:%v default:%v}",
					p.Name, p.Type, p.Multiple, p.Default, w.kind, w.multiple, w.def)
			}
		}
	})

	t.Run("REPORT_CONNECTION is not imported", func(t *testing.T) {
		if _, found := res.Dataset.Param("report_connection"); found {
			t.Error("imported Jasper's own JDBC connection as a parameter")
		}
	})

	t.Run("roles come from the report's own totals", func(t *testing.T) {
		total, ok := res.Dataset.Field("total")
		if !ok {
			t.Fatal("no total field")
		}
		if total.Role != "measure" || total.Aggregate != "sum" {
			t.Errorf("total = %s/%s, want measure/sum — the report summed it", total.Role, total.Aggregate)
		}
		// Grouped on, so a dimension whatever its class.
		if name, _ := res.Dataset.Field("customer_name"); name.Role != "dimension" {
			t.Errorf("customer_name = %s, want dimension", name.Role)
		}
		// Numeric, but named like an identifier and never totalled.
		id, _ := res.Dataset.Field("invoice_id")
		if id.Role != "dimension" {
			t.Errorf("invoice_id = %s, want dimension — an id is not a quantity", id.Role)
		}
	})

	t.Run("labels come from the column header", func(t *testing.T) {
		for name, want := range map[string]string{
			"issued_at": "Issued", "status": "Status", "total": "Amount",
			"customer_name": "Customer", // from fieldDescription, not the header
		} {
			f, _ := res.Dataset.Field(name)
			if f.Label != want {
				t.Errorf("field %q label = %q, want %q", name, f.Label, want)
			}
		}
	})

	t.Run("the table is the detail band, in page order", func(t *testing.T) {
		out, ok := res.Report.Output("pdf")
		if !ok {
			t.Fatal("no pdf output")
		}
		table := blockOfKind(t, out.Layout, "table")
		want := []string{"customer_name", "issued_at", "status", "total"}
		if strings.Join(table.Columns, ",") != strings.Join(want, ",") {
			t.Errorf("columns = %v, want %v", table.Columns, want)
		}
		if table.GroupBy != "customer_name" {
			t.Errorf("groupBy = %q, want customer_name", table.GroupBy)
		}
		if table.PageBreak != "perGroup" {
			t.Errorf("pageBreak = %q, want perGroup — the group set isStartNewPage", table.PageBreak)
		}
		if strings.Join(table.Subtotals, ",") != "total" {
			t.Errorf("subtotals = %v, want [total]", table.Subtotals)
		}
		if len(table.Sort) != 1 || table.Sort[0].Field != "issued_at" || table.Sort[0].Dir != "desc" {
			t.Errorf("sort = %+v, want issued_at desc", table.Sort)
		}
	})

	t.Run("the page is the paper it printed on", func(t *testing.T) {
		out, _ := res.Report.Output("pdf")
		if out.Page.Size != "A4" {
			t.Errorf("page size = %q, want A4 for 595x842 points", out.Page.Size)
		}
		if out.Page.Orientation != "portrait" {
			t.Errorf("orientation = %q, want portrait", out.Page.Orientation)
		}
		if out.Page.Margins != "7.1mm" {
			t.Errorf("margins = %q, want 7.1mm for 20 points", out.Page.Margins)
		}
		if out.Footer.Text != "Page {{ .page }} of {{ .pages }}" {
			t.Errorf("footer = %q, want the running page number", out.Footer.Text)
		}
	})

	t.Run("the title becomes a heading", func(t *testing.T) {
		out, _ := res.Report.Output("pdf")
		text := blockOfKind(t, out.Layout, "text")
		if text.Text != "Monthly Invoice Statement" || text.Style != "h1" {
			t.Errorf("title block = %q/%q", text.Style, text.Text)
		}
		if res.Report.Title != "Monthly Invoice Statement" {
			t.Errorf("report title = %q", res.Report.Title)
		}
	})

	t.Run("the report total becomes a stat", func(t *testing.T) {
		out, ok := res.Report.Output("interactive")
		if !ok {
			t.Fatal("no interactive output")
		}
		stat := blockOfKind(t, out.Layout, "stat")
		if stat.Value.Field != "total" || stat.Value.Aggregate != "sum" {
			t.Errorf("stat = %+v, want sum of total", stat.Value)
		}
		// The caption the summary band printed beside the number, not the
		// column heading: "Total billed" describes the total, "Amount"
		// describes a row.
		if stat.Label != "Total billed" {
			t.Errorf("stat label = %q, want the caption beside it in the summary band", stat.Label)
		}
	})

	t.Run("names are cronos names", func(t *testing.T) {
		if res.Dataset.Name != "monthly-invoice-statement" {
			t.Errorf("dataset name = %q", res.Dataset.Name)
		}
		if res.Report.Dataset != res.Dataset.Name {
			t.Errorf("report reads %q, dataset is %q", res.Report.Dataset, res.Dataset.Name)
		}
		if res.Source != "Monthly_Invoice_Statement" {
			t.Errorf("source = %q, want the name as the file spelled it", res.Source)
		}
	})

	t.Run("nothing is blocked", func(t *testing.T) {
		if res.Blocked() {
			t.Errorf("a report that imports cleanly was reported blocked:\n%s", render(res))
		}
	})
}

// TestImportedDefinitionsLoad is the assertion the rest of the feature rests on:
// what the importer writes, the engine reads. Encoding and re-loading catches
// what a struct comparison cannot — a field the codec refuses, a name the
// validator rejects, a template the compiler cannot bind.
func TestImportedDefinitionsLoad(t *testing.T) {
	res := importFixture(t, "statement.jrxml")

	rawDS, err := (codec.Encoder{}).Dataset(res.Dataset)
	if err != nil {
		t.Fatalf("encode dataset: %v", err)
	}
	ds, err := (codec.Loader{}).Dataset(rawDS)
	if err != nil {
		t.Fatalf("load dataset back: %v\n%s", err, rawDS)
	}
	// The check publish runs, which is what proves the query's templates bind.
	if err := query.Check(ds); err != nil {
		t.Errorf("imported dataset does not compile: %v\n%s", err, rawDS)
	}

	rawReport, err := (codec.Encoder{}).Report(res.Report)
	if err != nil {
		t.Fatalf("encode report: %v", err)
	}
	if _, err := (codec.Loader{}).Report(rawReport); err != nil {
		t.Fatalf("load report back: %v\n%s", err, rawReport)
	}

	// Every column and every measure a block names has to be a field of the
	// dataset, or the report loads and fails when somebody opens it.
	for _, out := range res.Report.Outputs {
		for i, b := range out.Layout {
			for _, name := range append(append([]string{}, b.Columns...), b.Subtotals...) {
				if _, ok := ds.Field(name); !ok {
					t.Errorf("%s block %d names column %q, which the dataset does not publish", out.Name, i, name)
				}
			}
			for _, ref := range []string{b.Value.Field, b.X.Field, b.Y.Field, b.GroupBy} {
				if ref == "" {
					continue
				}
				if _, ok := ds.Field(ref); !ok {
					t.Errorf("%s block %d references %q, which the dataset does not publish", out.Name, i, ref)
				}
			}
		}
	}
}

func importFixture(t *testing.T, name string) Result {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	res, err := (Importer{DataSource: "warehouse", Folder: "/imported"}).Import(data)
	if err != nil {
		t.Fatalf("import %s: %v", name, err)
	}
	return res
}

func blockOfKind(t *testing.T, layout []definition.Block, kind string) definition.Block {
	t.Helper()
	for _, b := range layout {
		if string(b.Kind) == kind {
			return b
		}
	}
	t.Fatalf("no %s block in layout", kind)
	return definition.Block{}
}

func render(r Result) string {
	var b strings.Builder
	for _, f := range r.Findings {
		b.WriteString("  " + f.String() + "\n")
	}
	return b.String()
}
