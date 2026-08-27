package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/store/file"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

// TestEstate runs the importer over the thing it was built for.
//
// Every other test here reads one file or six. The tool is pointed at four
// hundred, and the behaviours that only exist at that size are the ones nobody
// checks: whether identical queries actually collapse into one dataset, whether
// two files whose names normalise together are kept apart, whether a refused
// file stops the run or is counted and stepped over, and whether the directory
// that comes out the other end is one the server will load.
//
// Two hundred files rather than four hundred, because the behaviours are the
// same and the test has to stay quick. The shapes are deliberate: three queries
// shared across the estate, a name that repeats, and files that cannot import
// at all.
func TestEstate(t *testing.T) {
	dir := t.TempDir()
	const files = 200
	wantBlocked := writeEstate(t, dir, files)

	out := filepath.Join(t.TempDir(), "definitions")
	im := newImporter(out, true)
	if err := im.walk([]string{dir}); err != nil {
		t.Fatalf("an estate with unimportable files in it stopped the run: %v", err)
	}

	t.Run("a refused file is counted, not fatal", func(t *testing.T) {
		if im.files != files {
			t.Errorf("read %d of %d files", im.files, files)
		}
		if im.blocked != wantBlocked {
			t.Errorf("blocked = %d, want %d", im.blocked, wantBlocked)
		}
	})

	t.Run("identical queries become one dataset", func(t *testing.T) {
		// The estate holds exactly three distinct queries, so it holds exactly
		// three datasets however many reports read them. This is the whole
		// argument for the format: forty copies of one governed query is the
		// legacy mistake, and an importer that reproduces it faithfully has
		// imported the mistake.
		got := names(t, filepath.Join(out, "datasets"))
		if len(got) != 3 {
			t.Errorf("wrote %d datasets for three distinct queries: %v", len(got), got)
		}
		if im.shared == 0 {
			t.Error("nothing was reported as sharing a dataset")
		}
	})

	t.Run("colliding names are kept apart", func(t *testing.T) {
		reports := names(t, filepath.Join(out, "reports"))
		seen := map[string]bool{}
		for _, r := range reports {
			if seen[r] {
				t.Errorf("two reports were written to %s", r)
			}
			seen[r] = true
		}
		if im.renamed == 0 {
			t.Error("the repeated report name did not produce a rename")
		}
		if len(reports) != files-wantBlocked {
			t.Errorf("wrote %d reports for %d importable files", len(reports), files-wantBlocked)
		}
	})

	t.Run("the server loads the whole estate", func(t *testing.T) {
		repo, err := file.Load(out)
		if err != nil {
			t.Fatalf("the repository refused the imported estate: %v", err)
		}
		compiled := map[string]bool{}
		blocks := 0
		for _, r := range repo.Reports() {
			ds, err := repo.Dataset(context.Background(), r.Dataset)
			if err != nil {
				t.Errorf("report %q reads dataset %q, which is not there: %v", r.Name, r.Dataset, err)
				continue
			}
			if !compiled[ds.Name] {
				compiled[ds.Name] = true
				if err := query.Check(ds); err != nil {
					t.Errorf("dataset %q does not compile: %v", ds.Name, err)
				}
			}
			blocks += checkReferences(t, r, ds)
		}
		if blocks == 0 {
			t.Error("no blocks were checked; the estate is not being read")
		}
	})
}

// checkReferences asserts every column a block names is a field of its dataset,
// which is what a dangling rename would break and nothing else would notice
// until somebody opened the report.
func checkReferences(t *testing.T, r definition.Report, ds definition.Dataset) int {
	t.Helper()
	n := 0
	for _, o := range r.Outputs {
		for i, b := range o.Layout {
			if b.Kind == definition.TextBlock {
				continue
			}
			named := append(append([]string{}, b.Columns...), b.Subtotals...)
			named = append(named, b.Value.Field, b.X.Field, b.Y.Field, b.GroupBy)
			for _, name := range named {
				if name == "" {
					continue
				}
				if _, ok := ds.Field(name); !ok {
					t.Errorf("%s/%s block %d names %q, which dataset %q does not publish",
						r.Name, o.Name, i, name, ds.Name)
				}
			}
			n++
		}
	}
	return n
}

// heading turns a field name into the label a column header would carry.
func heading(field string) string {
	words := strings.Split(field, "_")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// estateQueries are the three governed queries the synthetic estate shares.
var estateQueries = []struct {
	sql    string
	fields [][2]string
}{
	{"SELECT c.name AS customer_name, i.issued_at, i.status, i.total\n" +
		"FROM invoices i JOIN customers c ON c.id=i.customer_id\n" +
		"WHERE i.issued_at BETWEEN $P{FROM_DATE} AND $P{TO_DATE}",
		[][2]string{{"customer_name", "java.lang.String"}, {"issued_at", "java.sql.Date"},
			{"status", "java.lang.String"}, {"total", "java.math.BigDecimal"}}},
	{"SELECT region, month, revenue, orders FROM sales WHERE month >= $P{FROM_DATE}",
		[][2]string{{"region", "java.lang.String"}, {"month", "java.lang.String"},
			{"revenue", "java.math.BigDecimal"}, {"orders", "java.lang.Integer"}}},
	{"SELECT depot, shipped_at, weight_kg, cost FROM shipments WHERE shipped_at >= $P{FROM_DATE}",
		[][2]string{{"depot", "java.lang.String"}, {"shipped_at", "java.sql.Date"},
			{"weight_kg", "java.math.BigDecimal"}, {"cost", "java.math.BigDecimal"}}},
}

// writeEstate lays out n reports across four folders and returns how many of
// them cannot import.
func writeEstate(t *testing.T, root string, n int) (blocked int) {
	t.Helper()
	folders := []string{"finance", "logistics", "hr", "ops"}
	for _, f := range folders {
		if err := os.MkdirAll(filepath.Join(root, f), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < n; i++ {
		doc, cannot := estateReport(i)
		if cannot {
			blocked++
		}
		path := filepath.Join(root, folders[i%len(folders)], fmt.Sprintf("report_%03d.jrxml", i))
		if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return blocked
}

func estateReport(i int) (doc string, cannot bool) {
	q := estateQueries[i%len(estateQueries)]
	sql, lang := q.sql, ""
	// Every twenty-fifth splices a parameter into its SQL, and every
	// thirty-seventh is not SQL at all. Both are refused, and the run has to
	// step over them rather than stop.
	switch {
	case i%25 == 0:
		sql, cannot = "SELECT id FROM t ORDER BY $P!{SORT_COL}", true
	case i%37 == 0:
		sql, lang, cannot = "from Invoice i", ` language="hql"`, true
	}
	// Every fortieth carries a name the others also use.
	name := fmt.Sprintf("Report_%d", i)
	if i%40 == 0 {
		name = "Shared_Report"
	}

	var fields, heads, detail strings.Builder
	for j, f := range q.fields {
		fmt.Fprintf(&fields, `<field name=%q class=%q/>`, f[0], f[1])
		fmt.Fprintf(&heads, `<staticText><reportElement x="%d" y="0" width="100" height="20"/>`+
			`<text><![CDATA[%s]]></text></staticText>`, j*110, heading(f[0]))
		fmt.Fprintf(&detail, `<textField><reportElement x="%d" y="0" width="100" height="20"/>`+
			`<textFieldExpression><![CDATA[$F{%s}]]></textFieldExpression></textField>`, j*110, f[0])
	}
	group := ""
	if i%3 == 0 {
		group = fmt.Sprintf(`<group name="G" isStartNewPage="true"><groupExpression>`+
			`<![CDATA[$F{%s}]]></groupExpression></group>`+
			`<variable name="GT" class="java.math.BigDecimal" resetType="Group" resetGroup="G" `+
			`calculation="Sum"><variableExpression><![CDATA[$F{%s}]]></variableExpression></variable>`,
			q.fields[0][0], q.fields[len(q.fields)-1][0])
	}
	// Every eleventh has a crosstab, so most of the estate carries a finding.
	extra := ""
	if i%11 == 0 {
		extra = `<summary><band height="100"><crosstab><rowGroup name="r"/></crosstab></band></summary>`
	}

	return `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport xmlns="http://jasperreports.sourceforge.net/jasperreports" name="` + name + `"
  pageWidth="595" pageHeight="842" leftMargin="20" rightMargin="20" topMargin="20" bottomMargin="20">
<parameter name="FROM_DATE" class="java.util.Date"><defaultValueExpression><![CDATA[new Date()]]></defaultValueExpression></parameter>
<parameter name="TO_DATE" class="java.util.Date"/>
<queryString` + lang + `><![CDATA[` + sql + `]]></queryString>
` + fields.String() + group + `
<columnHeader><band height="20">` + heads.String() + `</band></columnHeader>
<detail><band height="20">` + detail.String() + `</band></detail>` + extra + `
</jasperReport>`, cannot
}
