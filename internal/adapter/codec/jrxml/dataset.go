package jrxml

import (
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// reserved are the parameters JasperReports supplies itself.
//
// Not imported: they are the engine's own plumbing — the JDBC connection, the
// locale, the parameter map, the scriptlet handle — and a cronos dataset has no
// equivalent for any of them. A report that reads one in its query is refused
// rather than imported without it, because the query would not compile.
var reserved = map[string]bool{
	"REPORT_CONNECTION": true, "REPORT_DATA_SOURCE": true, "REPORT_PARAMETERS_MAP": true,
	"REPORT_LOCALE": true, "REPORT_RESOURCE_BUNDLE": true, "REPORT_TIME_ZONE": true,
	"REPORT_SCRIPTLET": true, "REPORT_VIRTUALIZER": true, "REPORT_CLASS_LOADER": true,
	"REPORT_URL_HANDLER_FACTORY": true, "REPORT_FILE_RESOLVER": true,
	"REPORT_FORMAT_FACTORY": true, "REPORT_MAX_COUNT": true, "REPORT_TEMPLATES": true,
	"REPORT_CONTEXT": true, "REPORT_COMPARATOR": true, "IS_IGNORE_PAGINATION": true,
	"JASPER_REPORT": true, "JASPER_REPORTS_CONTEXT": true, "SORT_FIELDS": true,
	"FILTER": true, "SUBREPORT_DIR": true,
}

// params translates the report's parameters, and returns the map from Jasper
// names to the identifiers the dataset declares.
//
// The map is what translateQuery needs: `$P{FROM_DATE}` has to become
// `{{ .params.from_date }}`, and the two normalisations must be the same one.
func (t *translation) params() ([]definition.Param, map[string]string) {
	var (
		out   []definition.Param
		names = map[string]string{}
		taken unique
	)
	for _, p := range t.doc.Parameters {
		if p.Name == "" {
			continue
		}
		if reserved[p.Name] {
			// Recorded in the map so a query reading one produces "cronos has no
			// equivalent" rather than "the report does not declare it".
			names[p.Name] = ""
			continue
		}
		name, renamed := taken.pick(paramName(p.Name), '_')
		if renamed {
			t.found.addf(Review, "parameter",
				"two parameters normalise to the same name, so %q was imported as %q — check the query binds the one you meant",
				p.Name, name)
		}
		names[p.Name] = name
		out = append(out, t.param(p, name))
	}
	return out, names
}

func (t *translation) param(p parameter, name string) definition.Param {
	kind, multiple, known := paramType(p.Class, p.NestedType)
	if !known {
		// A parameter of an unmapped class still has to exist, or the query that
		// reads it will not compile. String binds anything a caller can send in
		// JSON, so the definition loads and the author retypes one line.
		kind = definition.String
		t.found.addf(Review, "parameter",
			"parameter %q is a %s, which has no cronos type — imported as a string, so check what it binds to",
			p.Name, orUnknown(p.Class))
	}

	out := definition.Param{
		Name: name, Type: kind, Multiple: multiple,
		Label: strings.TrimSpace(p.Description),
	}
	t.setDefault(&out, p)
	return out
}

// setDefault carries a Jasper default value across when it is a value.
//
// Required is the interesting consequence. Jasper prompts for a parameter with
// no default, so `required: true` is the faithful import — but only when the
// import is confident there was no default, rather than when it could not read
// one. A default it cannot read leaves the parameter optional with a finding,
// because marking it required would break every caller that used to rely on the
// report computing it.
func (t *translation) setDefault(out *definition.Param, p parameter) {
	expr := strings.TrimSpace(p.Default)
	if expr == "" {
		out.Required = p.prompts()
		if !p.prompts() {
			t.found.addf(Review, "parameter",
				"parameter %q was computed by the report rather than asked for, and has no default here — a caller now has to supply it",
				p.Name)
		}
		return
	}
	value, ok := javaDefault(expr)
	if !ok {
		t.found.addf(Review, "defaultValueExpression",
			"parameter %q defaulted to the Java expression %s, which cannot be a cronos default — the parameter is optional with no default",
			p.Name, oneLine(expr))
		return
	}
	// An enum's values are the only thing that constrains it, and a Jasper
	// parameter has no value list — so a default alone stays untyped rather
	// than inventing an enum.
	out.Default = value
}

// fields translates the query's columns.
//
// Roles are the substance here: a cronos measure must declare how to fold
// itself, and nothing in a `.jrxml` says which columns are quantities. The
// evidence is the variables — a `<variable calculation="Sum">` over `$F{total}`
// is the report telling us `total` is a measure and that sum is what it meant —
// and where there is no evidence, the class and the name decide.
func (t *translation) fields() []definition.Field {
	evidence := t.measureEvidence()
	grouped := t.groupedFields()

	var (
		out     []definition.Field
		taken   unique
		named   = map[string]string{}
		guessed int
	)
	for _, f := range t.doc.Fields {
		if f.Name == "" {
			continue
		}
		name := t.fieldNamed(f, &taken)
		named[f.Name] = name

		field := t.field(f, name)
		if t.role(&field, f, evidence, grouped) {
			guessed++
		}
		out = append(out, field)
	}
	t.fieldNames = named
	if guessed > 0 {
		t.found.addN(Note, "field",
			"a numeric column with no total in the original was imported as a measure that sums, "+
				"because nothing in a .jrxml says which columns are quantities — check the ones that are not",
			guessed)
	}
	return out
}

// fieldNamed resolves the cronos name for a Jasper field, reporting both ways it
// can go wrong.
func (t *translation) fieldNamed(f field, taken *unique) string {
	base, clean := fieldName(f.Name)
	if !clean {
		t.found.addf(Review, "field",
			"field %q cannot be a cronos field name and was imported as %q — the query must return a column called that, so alias it if it does not",
			f.Name, base)
	}
	name, renamed := taken.pick(base, '_')
	if renamed {
		t.found.addf(Review, "field",
			"two fields normalise to the same name, so %q was imported as %q", f.Name, name)
	}
	return name
}

// field translates a Jasper field's declared type, leaving its role to role().
func (t *translation) field(f field, name string) definition.Field {
	kind, known := fieldType(f.Class)
	if !known {
		kind = "string"
		t.found.addf(Review, "field",
			"field %q is a %s, which is not a value a report can display — imported as a string",
			f.Name, orUnknown(f.Class))
	}
	return definition.Field{
		Name: name, Type: kind, Label: strings.TrimSpace(f.Description),
		Role: definition.Dimension,
	}
}

// role decides whether a column is something to group by or something to add up,
// and reports whether it had to guess.
//
// Evidence first, in both directions: a column the report totalled is a measure
// and the fold it used is the one it meant, and a column the report grouped by is
// a dimension whatever its Java class. Only a numeric column the report neither
// totalled nor grouped by is a guess.
func (t *translation) role(into *definition.Field, f field,
	evidence map[string]string, grouped map[string]bool) (guessed bool) {

	switch agg, isMeasure := evidence[f.Name]; {
	case isMeasure:
		into.Role, into.Aggregate = definition.Measure, agg
	case grouped[f.Name]:
		// Grouped on, so a dimension whatever its class.
	case (into.Type == "number" || into.Type == "decimal") && !looksLikeDimension(f.Name):
		into.Role, into.Aggregate = definition.Measure, "sum"
		return true
	}
	return false
}

// measureEvidence reads the variables for fields the report totalled, and the
// fold it used.
func (t *translation) measureEvidence() map[string]string {
	out := map[string]string{}
	for _, v := range t.doc.Variables {
		r, plain := plainRef(v.Expression)
		if !plain || r.sigil != 'F' {
			continue
		}
		agg, ok := aggregateOf(v.Calculation)
		if !ok {
			continue
		}
		if was, seen := out[r.name]; seen && was != agg {
			// Two variables folding one column two ways. The dataset's field
			// carries one default, and a block may still override it.
			t.found.addf(Review, "variable",
				"field %q was both %sed and %sed in the original; the dataset declares %s and a block can override it",
				r.name, was, agg, was)
			continue
		}
		out[r.name] = agg
	}
	return out
}

// groupedFields reads the group expressions for fields the report broke on.
func (t *translation) groupedFields() map[string]bool {
	out := map[string]bool{}
	for _, g := range t.doc.Groups {
		if r, plain := plainRef(g.Expression); plain && r.sigil == 'F' {
			out[r.name] = true
		}
	}
	return out
}

// rowScope is deliberately empty.
//
// A Jasper report has no row-level security to carry: JasperReports Server
// enforces access at the report level and by passing a parameter into the
// query, so there is nothing in the file that means "these rows, for this
// caller". Emitting a predicate would be inventing a security rule, and
// emitting none is the truthful import — but a dataset with no rowLevelSecurity
// is readable in full by anyone who can reach the report, which is worth saying
// once per file rather than discovering per deployment.
func (t *translation) rowScope() []definition.RowScope {
	t.found.add(Note, "rowLevelSecurity",
		"a .jrxml carries no row-level security, so the dataset has none and returns every row "+
			"the query selects — add a predicate if this report was restricted by who ran it")
	return nil
}

func orUnknown(class string) string {
	if strings.TrimSpace(class) == "" {
		return "field with no class"
	}
	return class
}

// oneLine flattens an expression for a finding, so a multi-line Java expression
// does not break the shape of a report someone is scanning.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const most = 80
	if len(s) > most {
		return s[:most] + "…"
	}
	return s
}

// checkQuotedAliases warns about the one rename this import cannot make safe.
//
// A cronos field name is lower case and reaches SQL as text, so `KundeName`
// imports as `kundename` and the compiled block selects that. Which is correct
// when the query wrote `AS KundeName` — Postgres folds an unquoted alias, and
// MySQL, SQLite and SQL Server match case-insensitively — and wrong when it
// wrote `AS "KundeName"`, because a quoted alias is only found by that exact
// spelling and cronos cannot spell it.
//
// Advisory rather than a refusal: a double-quoted string is a literal rather
// than an identifier in MySQL's default mode, so this can be a false positive,
// and it costs a reader one line to dismiss. The alternative is a report that
// loads, passes review, and returns "column kundename does not exist" the first
// time somebody opens it.
func (t *translation) checkQuotedAliases(sql string, fields []definition.Field) {
	declared := make(map[string]bool, len(fields))
	for _, f := range fields {
		declared[f.Name] = true
	}
	for _, quoted := range doubleQuoted(sql) {
		if quoted == strings.ToLower(quoted) || !declared[strings.ToLower(quoted)] {
			continue
		}
		t.found.addf(Review, "queryString",
			"the query aliases a column as %q with quotes, which only that exact spelling finds; "+
				"the field imported as %q, so drop the quotes or alias it in lower case",
			quoted, strings.ToLower(quoted))
	}
}

// doubleQuoted returns the double-quoted runs in a statement.
func doubleQuoted(sql string) []string {
	var out []string
	for i := 0; i < len(sql); i++ {
		if sql[i] != '"' {
			continue
		}
		end := strings.IndexByte(sql[i+1:], '"')
		if end < 0 {
			break
		}
		out = append(out, sql[i+1:i+1+end])
		i += end + 1
	}
	return out
}
