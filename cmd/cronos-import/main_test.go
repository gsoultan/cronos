package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/codec/jrxml"
	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
)

// TestSharesOneDatasetAcrossReports is the reason -share-datasets exists.
//
// docs/report-format.md argues that one query per report is the legacy mistake:
// no reuse, security copy-pasted per report, nothing to govern. An importer that
// faithfully produced four hundred copies of one query would have imported the
// mistake along with the meaning — so identical queries become one dataset, and
// the reports point at it.
func TestSharesOneDatasetAcrossReports(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "north.jrxml", report("Sales_North"))
	write(t, dir, "south.jrxml", report("Sales_South"))
	out := filepath.Join(t.TempDir(), "definitions")

	im := newImporter(out, true)
	if err := im.walk([]string{dir}); err != nil {
		t.Fatal(err)
	}

	datasets := names(t, filepath.Join(out, "datasets"))
	if len(datasets) != 1 {
		t.Errorf("wrote %d datasets for two identical queries: %v", len(datasets), datasets)
	}
	reports := names(t, filepath.Join(out, "reports"))
	if len(reports) != 2 {
		t.Errorf("wrote %d reports, want 2: %v", len(reports), reports)
	}
	for _, r := range reports {
		body := read(t, filepath.Join(out, "reports", r))
		if !strings.Contains(body, "dataset: "+strings.TrimSuffix(datasets[0], ".yaml")) {
			t.Errorf("%s does not read the shared dataset:\n%s", r, body)
		}
	}
	if im.shared != 1 {
		t.Errorf("shared = %d, want 1", im.shared)
	}
	// The security warning counts datasets to fix, not files read. Two reports
	// over one shared query are one predicate to write.
	if im.unscoped != 1 {
		t.Errorf("unscoped = %d, want 1 — a shared dataset is counted once", im.unscoped)
	}
}

// TestKeepsDifferentQueriesApart is the other half: sharing must be by content,
// or two reports that ask different questions end up bound to one answer.
func TestKeepsDifferentQueriesApart(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.jrxml", report("Sales_North"))
	write(t, dir, "b.jrxml", strings.Replace(report("Sales_South"),
		"FROM sales", "FROM sales WHERE region = 'south'", 1))
	out := filepath.Join(t.TempDir(), "definitions")

	if err := newImporter(out, true).walk([]string{dir}); err != nil {
		t.Fatal(err)
	}
	if got := names(t, filepath.Join(out, "datasets")); len(got) != 2 {
		t.Errorf("wrote %d datasets for two different queries: %v", len(got), got)
	}
}

// TestNamesDoNotCollide covers two files whose names normalise to one. Silently
// overwriting the first would lose a report and report success.
func TestNamesDoNotCollide(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "one.jrxml", report("Sales Summary"))
	write(t, dir, "two.jrxml", strings.Replace(report("sales_summary"),
		"FROM sales", "FROM sales_archive", 1))
	out := filepath.Join(t.TempDir(), "definitions")

	im := newImporter(out, true)
	if err := im.walk([]string{dir}); err != nil {
		t.Fatal(err)
	}
	reports := names(t, filepath.Join(out, "reports"))
	if len(reports) != 2 {
		t.Fatalf("two reports with colliding names produced %d files: %v", len(reports), reports)
	}
	if len(names(t, filepath.Join(out, "datasets"))) != 2 {
		t.Error("two different queries were merged by a name collision")
	}
	// Said out loud, because the author will go looking for a name that is not
	// there.
	if im.renamed == 0 {
		t.Error("a definition was renamed to avoid a collision without reporting it")
	}
}

// TestRerunIsIdempotent is what makes the tool safe to run twice: an import over
// its own output changes nothing and does not need -force.
func TestRerunIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.jrxml", report("Sales_North"))
	out := filepath.Join(t.TempDir(), "definitions")

	if err := newImporter(out, true).walk([]string{dir}); err != nil {
		t.Fatal(err)
	}
	before := read(t, filepath.Join(out, "datasets", "sales-north.yaml"))

	second := newImporter(out, true)
	if err := second.walk([]string{dir}); err != nil {
		t.Fatalf("re-running the import over its own output failed: %v", err)
	}
	if after := read(t, filepath.Join(out, "datasets", "sales-north.yaml")); after != before {
		t.Error("a second run rewrote the definition")
	}
	if second.wrote != 0 {
		t.Errorf("second run reported writing %d definitions, want 0", second.wrote)
	}
}

// TestRefusesToClobber is the safety this needs to be pointed at a live
// definitions directory: a file that exists and differs stops the run.
func TestRefusesToClobber(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.jrxml", report("Sales_North"))
	out := filepath.Join(t.TempDir(), "definitions")

	if err := os.MkdirAll(filepath.Join(out, "datasets"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(out, "datasets", "sales-north.yaml")
	if err := os.WriteFile(existing, []byte("apiVersion: cronos.dev/v1\n# hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := newImporter(out, true).walk([]string{dir})
	if err == nil {
		t.Fatal("overwrote a definition that already existed and differed")
	}
	if !strings.Contains(err.Error(), "-force") {
		t.Errorf("the refusal does not name the way past it: %v", err)
	}
	if body := read(t, existing); !strings.Contains(body, "hand written") {
		t.Error("the existing definition was overwritten anyway")
	}

	forced := newImporter(out, true)
	forced.force = true
	if err := forced.walk([]string{dir}); err != nil {
		t.Fatalf("-force did not get past it: %v", err)
	}
	if body := read(t, existing); strings.Contains(body, "hand written") {
		t.Error("-force did not overwrite")
	}
}

// TestDryRunWritesNothingAndSaysWhatItWould is the default mode, and the answer
// to the first question anyone asks a migration tool.
func TestDryRunWritesNothingAndSaysWhatItWould(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.jrxml", report("Sales_North"))
	out := filepath.Join(t.TempDir(), "definitions")

	im := newImporter("", true)
	if err := im.walk([]string{dir}); err != nil {
		t.Fatal(err)
	}
	if im.wrote != 2 {
		t.Errorf("a dry run counted %d definitions, want 2 — the safe mode has to answer the question", im.wrote)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("a dry run created the output directory")
	}
}

func newImporter(out string, share bool) *importer {
	return &importer{
		from:  jrxml.Importer{DataSource: "warehouse"},
		out:   out,
		share: share,
	}
}

// report is a minimal but valid .jrxml, parameterised by name.
func report(name string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<jasperReport xmlns="http://jasperreports.sourceforge.net/jasperreports" name="` + name + `">
	<queryString><![CDATA[SELECT region, revenue FROM sales]]></queryString>
	<field name="region" class="java.lang.String"/>
	<field name="revenue" class="java.math.BigDecimal"/>
	<detail><band height="20">
		<textField><reportElement x="0" y="0" width="100" height="20"/>
			<textFieldExpression><![CDATA[$F{region}]]></textFieldExpression></textField>
		<textField><reportElement x="110" y="0" width="100" height="20"/>
			<textFieldExpression><![CDATA[$F{revenue}]]></textFieldExpression></textField>
	</band></detail>
</jasperReport>`
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// TestWarnsTheDirectoryIsNotRunnable covers the last thing standing between an
// imported estate and a report that renders.
//
// Every imported dataset reads the -datasource, and a .jrxml names no database,
// so the importer cannot write that definition. Noticing it is absent is the
// next best thing, and it has to happen here rather than at a publish that
// fails or a schedule that fires.
func TestWarnsTheDirectoryIsNotRunnable(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.jrxml", report("Sales_North"))
	out := filepath.Join(t.TempDir(), "definitions")

	im := newImporter(out, true)
	if err := im.walk([]string{dir}); err != nil {
		t.Fatal(err)
	}
	if im.hasDataSource("warehouse") {
		t.Fatal("claimed a datasource the importer never wrote")
	}

	// Writing the definition it suggests makes the warning stop, which is what
	// makes it a work item rather than a permanent scold.
	if err := os.MkdirAll(filepath.Join(out, "datasources"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(out, "datasources"), "warehouse.yaml", dataSourceTemplate("warehouse"))
	if !im.hasDataSource("warehouse") {
		t.Error("the datasource it told the operator to write is not one it then recognises")
	}
}

// TestSuggestedDataSourceLoads: a template that does not parse is worse than no
// template, because it is read as authoritative and the error it produces names
// the operator's file rather than this one.
func TestSuggestedDataSourceLoads(t *testing.T) {
	ds, err := (codec.Loader{}).DataSource([]byte(dataSourceTemplate("warehouse")))
	if err != nil {
		t.Fatalf("the DataSource template cronos-import prints does not load: %v", err)
	}
	if ds.Name != "warehouse" || ds.Driver != "postgres" {
		t.Errorf("template loaded as %+v", ds)
	}
	if ds.Limits.MaxRows == 0 || ds.Limits.StatementTimeout == 0 {
		// The two limits AGENTS.md requires of every datasource. A template
		// without them teaches the unbounded version.
		t.Errorf("the template omits a row cap or a statement timeout: %+v", ds.Limits)
	}
}
