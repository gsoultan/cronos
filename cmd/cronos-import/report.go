package main

import (
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/adapter/codec/jrxml"
	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/adapter/store/file"
)

// Everything the run says out loud.
//
// Separated from the work because the wording is the product here: an importer
// that carries most of a format is only usable if the part it did not carry is
// legible, and that is a different concern from reading the files.

func (im *importer) report(path string, res jrxml.Result, refused error) {
	var shown []jrxml.Finding
	notes := 0
	for _, f := range res.Findings {
		if f.Severity == jrxml.Note && !im.verbose {
			notes += max(f.Count, 1)
			continue
		}
		shown = append(shown, f)
	}
	if refused == nil && len(shown) == 0 {
		return
	}

	fmt.Println(path)
	if refused != nil && !res.Blocked() {
		// A refusal normally arrives as a blocking finding, which is printed
		// below. This is the belt for a refusal that somehow carried none.
		fmt.Printf("  blocked: %s\n", oneLine(refused.Error()))
	}
	for _, f := range shown {
		fmt.Printf("  %s\n", f)
	}
	if notes > 0 {
		fmt.Printf("  note: %d cosmetic difference%s — -v lists them\n", notes, plural(notes))
	}
	fmt.Println()
}

func (im *importer) summarise() {
	what := "would write"
	if im.out != "" {
		what = "wrote"
	}
	fmt.Printf("%d file%s · %s %d definition%s",
		im.files, plural(im.files), what, im.wrote, plural(im.wrote))
	if im.shared > 0 {
		fmt.Printf(" · %d report%s share a dataset already imported", im.shared, plural(im.shared))
	}
	fmt.Printf(" · %d to review · %d blocked\n", im.review, im.blocked)
	if im.renamed > 0 {
		// Two Jasper names that normalised to one. Said out loud because the
		// author will go looking for a name that is not there.
		fmt.Printf("%d definition%s renamed with a numeric suffix, because another file "+
			"normalised to the same name.\n", im.renamed, plural(im.renamed))
	}
	if im.out == "" {
		fmt.Println("\nNothing was written. Pass -out <dir> to write the definitions.")
	}
	if im.blocked > 0 {
		fmt.Printf("\n%d file%s need%s a person before %s migrated.\n",
			im.blocked, plural(im.blocked), was(im.blocked), they(im.blocked))
	}
	im.warnUnscoped()
	im.warnNoDataSource()
}

// warnNoDataSource says the imported directory is not yet runnable.
//
// Every imported dataset reads -datasource, and a .jrxml names no database, so
// this importer cannot write that DataSource: it does not know the driver, the
// host or which secret holds the password. What it can do is notice that the
// definition is not in the directory it just wrote, and say so here rather than
// let it be found by a publish that fails or, worse, a schedule that fires.
//
// Checked against the directory rather than assumed missing, so pointing the
// importer at a definitions tree that already has the datasource stays quiet.
func (im *importer) warnNoDataSource() {
	name := im.from.DataSource
	if im.out == "" || im.wrote == 0 || im.hasDataSource(name) {
		return
	}
	fmt.Printf("\n! No DataSource called %q in %s.\n", name, im.out)
	fmt.Printf(`  Every imported dataset reads it, and a .jrxml names no database — it carries
  no driver, no host and no credential — so this could not write one. Add it
  before publishing:

      # %s/datasources/%s.yaml
%s

  docs/report-format.md
`, im.out, name, indent(dataSourceTemplate(name), "      "))
}

// dataSourceTemplate is the definition the operator has to write, with the one
// field this cannot guess left as a choice.
//
// A function rather than a string in the Printf above so it can be loaded in a
// test. Printing a template that does not parse would be worse than printing
// nothing: it is read as authoritative, and the error it produces names the
// operator's file rather than this.
func dataSourceTemplate(name string) string {
	return `apiVersion: cronos.dev/v1
kind: DataSource
metadata:
  name: ` + name + `
spec:
  driver: postgres                  # or mysql, sqlserver, sqlite
  dsn: ${secret:` + name + `_dsn}
  limits: {statementTimeout: 30s, maxRows: 1000000}`
}

// indent shifts a block so it reads as a quotation rather than as output.
func indent(body, pad string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}

// hasDataSource reports whether the output directory already holds the
// datasource the imported datasets read.
func (im *importer) hasDataSource(name string) bool {
	repo, err := file.Load(im.out)
	if err != nil {
		// The directory holds something this cannot read. Staying quiet about
		// the datasource is right: the run has a louder problem than this.
		return true
	}
	for _, existing := range repo.Names(codec.KindDataSource) {
		if existing == name {
			return true
		}
	}
	return false
}

// warnUnscoped says the one thing in this output that is a security decision.
//
// It is printed once, last, and outside the per-file findings on purpose. Per
// file it would be a line on every report and read as boilerplate; as a count at
// the end it is a fact about the whole import. It is not an error and does not
// change the exit status — an imported estate with no predicates yet is the
// expected state on day one — but it is the sentence somebody has to read before
// they publish.
func (im *importer) warnUnscoped() {
	if im.unscoped == 0 {
		return
	}
	fmt.Printf("\n! %d dataset%s %s no row-level security.\n",
		im.unscoped, plural(im.unscoped), has(im.unscoped))
	fmt.Print(`  A .jrxml carries none — JasperReports Server enforced access above the
  report — so each of these returns every row its query selects to anybody who
  can reach it. Projects still isolate each other; this is the scope *within* a
  project. Add a predicate before publishing anything an end customer reaches:

      rowLevelSecurity:
        - predicate: customer_id = {{ .scope.customer_id }}

  docs/migrating-from-jasper.md · docs/tenancy.md
`)
}

// collect finds every .jrxml under the given paths, in a stable order.
