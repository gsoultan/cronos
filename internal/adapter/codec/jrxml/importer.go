package jrxml

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

// Importer translates JasperReports files into cronos definitions.
//
// Configuration is the two things a `.jrxml` cannot tell us. A Jasper report
// gets its connection from the server that runs it — a JDBC data adapter, or a
// connection handed in by the caller — so nothing in the file names a database,
// and the catalog folder it belonged to is a path in a repository this has never
// seen. Both are the operator's to supply, once, for the whole estate.
type Importer struct {
	// DataSource is the cronos DataSource every imported dataset reads. Required:
	// a dataset with no source does not validate, and guessing a name would
	// produce definitions that load and cannot run.
	DataSource string
	// Folder is where imported reports appear in the catalog. Optional.
	Folder string
}

// Import translates one JasperReports document.
//
// The error is for a file that produced nothing: unreadable XML, or a query
// whose meaning cannot be carried without changing it. Everything else — a
// dropped subreport, an unmappable chart, a layout that could not be inferred —
// comes back in Result.Findings, because a report with fourteen cosmetic losses
// and a working query is a successful import and refusing it would mean the tool
// only accepts files that did not need it.
//
// Result.Findings is populated even when the error is non-nil, so a caller
// triaging an estate can say why a file was refused rather than only that it was.
func (i Importer) Import(data []byte) (Result, error) {
	if strings.TrimSpace(i.DataSource) == "" {
		return Result{}, errors.New("jrxml: Importer.DataSource is required — " +
			"a .jrxml names no database, so the datasource its query reads has to be supplied")
	}

	doc, err := parse(data)
	if err != nil {
		return Result{}, err
	}

	t := &translation{doc: doc, opts: i}
	// The census first, so a file refused later still reports what was in it.
	census(data, &t.found)

	out := Result{Source: doc.Name}
	ds, err := t.dataset()
	if err != nil {
		out.Findings = t.found.sorted()
		return out, err
	}
	out.Dataset = ds
	out.Report = t.report(ds)
	out.Findings = t.found.sorted()
	return out, nil
}

// translation is one file being translated: the document, the options, and what
// has been found so far.
//
// A struct rather than threaded arguments because every step reports, and a
// findings accumulator passed to twenty functions is an argument nobody reads
// and one call site eventually forgets.
type translation struct {
	doc   document
	opts  Importer
	found findings

	// fieldNames maps a Jasper field name to the identifier the dataset
	// declares. Built by fields() and read by everything that resolves an
	// expression, which is why the dataset half runs first.
	fieldNames map[string]string
	// byName indexes the emitted fields, so a chart can ask whether the column
	// it plots is a measure.
	byName map[string]definition.Field
}

// refuse records a blocking finding and returns the error that carries it.
//
// One call rather than two so the two cannot disagree. The finding is what a
// migration run prints and counts; the error is what a programmatic caller
// checks. A refusal that produced only one of them would either be invisible in
// the report or uncounted in the summary.
func (t *translation) refuse(element, format string, args ...any) error {
	err := fmt.Errorf("%w: "+format, append([]any{ErrRefused}, args...)...)
	// Without the sentinel's own prefix: the report already says "blocked".
	t.found.add(Blocked, element, strings.TrimPrefix(err.Error(), ErrRefused.Error()+": "))
	return err
}

// dataset translates the governed query: the half of a `.jrxml` that is worth
// most and survives best.
func (t *translation) dataset() (definition.Dataset, error) {
	if err := t.checkQueryLanguage(); err != nil {
		return definition.Dataset{}, err
	}

	params, names := t.params()
	sql, err := t.query(names)
	if err != nil {
		return definition.Dataset{}, err
	}

	fields := t.fields()
	if len(fields) == 0 {
		return definition.Dataset{}, t.refuse("field",
			"the report declares no fields, so there is no dataset to describe — "+
				"a report fed by a subreport's data source rather than its own query imports as nothing")
	}
	t.byName = make(map[string]definition.Field, len(fields))
	for _, f := range fields {
		t.byName[f.Name] = f
	}
	t.checkQuotedAliases(sql, fields)
	name := t.name()

	ds := definition.Dataset{
		Name:             name,
		Title:            humanise(t.doc.Name),
		Description:      fmt.Sprintf("Imported from the JasperReports report %q.", t.doc.Name),
		Sources:          []definition.SourceRef{{Ref: t.opts.DataSource}},
		Query:            sql,
		Params:           params,
		Fields:           fields,
		RowLevelSecurity: t.rowScope(),
	}
	if err := ds.Validate(); err != nil {
		return definition.Dataset{}, t.refuse("jasperReport",
			"the imported dataset is not one cronos will store: %v", err)
	}
	// The same check publish runs. Catching it here means the author sees it
	// beside the file that caused it, rather than at 6am in a delivery log.
	if err := query.Check(ds); err != nil {
		return definition.Dataset{}, t.refuse("queryString",
			"the imported query will not compile: %v", err)
	}
	return ds, nil
}

// name is the definition name, and a finding when nothing survived making one.
func (t *translation) name() string {
	name := slugify(t.doc.Name)
	if name == fallbackName && !strings.EqualFold(t.doc.Name, fallbackName) {
		// Nothing in the name survived the reduction to a cronos identifier — a
		// report named in Cyrillic or Chinese, most likely. It imports, under a
		// name nobody will recognise in the catalog.
		t.found.addf(Review, "jasperReport",
			"the report is called %q, which has no letters a cronos name may hold; it imported as %q and wants renaming",
			t.doc.Name, name)
	}
	return name
}

// checkQueryLanguage refuses a query that is not SQL.
//
// HQL, MDX, XPath and the rest are not SQL with different keywords, and a
// dataset that claims to hold one when it holds another fails when it runs
// rather than when it is imported.
func (t *translation) checkQueryLanguage() error {
	lang := strings.TrimSpace(t.doc.Query.Language)
	if lang == "" || strings.EqualFold(lang, "sql") {
		return nil
	}
	if strings.EqualFold(lang, "plsql") {
		return t.refuse("queryString", "the query is PL/SQL, which means an Oracle database — "+
			"cronos drivers are postgres, mysql, sqlserver, sqlite and duckdb, so this report "+
			"needs its query and its database moved before it can be imported")
	}
	return t.refuse("queryString",
		"the query is written in %s rather than SQL, and cronos datasets are SQL", lang)
}

// query translates the SQL, binding the parameters it reads.
func (t *translation) query(names map[string]string) (string, error) {
	sql := strings.TrimSpace(t.doc.Query.SQL)
	if sql == "" {
		return "", t.refuse("queryString",
			"the report has no query — it was filled by whatever ran it, "+
				"which in cronos is a Dataset this file does not describe")
	}
	out, err := translateQuery(sql, names)
	if err != nil {
		// translateQuery already wraps ErrRefused with the explanation; this
		// puts the same sentence in the findings.
		t.found.add(Blocked, "queryString", strings.TrimPrefix(err.Error(), ErrRefused.Error()+": "))
		return "", err
	}
	// Trailing newline so the emitted YAML block scalar ends cleanly, which is
	// what every hand-written definition in the repository does.
	return strings.TrimRight(out, " \t\n") + "\n", nil
}

// allSections is every band-bearing slot in the document, groups included.
func (t *translation) allSections() []section {
	out := []section{
		t.doc.Title, t.doc.PageHeader, t.doc.ColumnHeader, t.doc.Detail,
		t.doc.ColumnFooter, t.doc.PageFooter, t.doc.LastPageFooter, t.doc.Summary,
	}
	for _, g := range t.doc.Groups {
		out = append(out, g.Header, g.Footer)
	}
	return out
}

// drawnVariables is the set of variables the report actually printed.
//
// A `.jrxml` accumulates variables nobody removed, and a stat tile for a total
// the original never showed is a number the author has to explain.
func (t *translation) drawnVariables() map[string]bool {
	out := map[string]bool{}
	for _, s := range t.allSections() {
		for _, f := range s.fields() {
			for _, r := range scanRefs(f.field.Expression) {
				if r.sigil == 'V' {
					out[r.name] = true
				}
			}
		}
	}
	return out
}

// humanise turns a Jasper identifier into something to put on a tile.
func humanise(s string) string {
	parts := words(s)
	if len(parts) == 0 {
		return s
	}
	joined := strings.Join(parts, " ")
	return strings.ToUpper(joined[:1]) + joined[1:]
}
