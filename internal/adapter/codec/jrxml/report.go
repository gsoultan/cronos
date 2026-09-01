package jrxml

import (
	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
One Jasper report becomes two outputs, and the second one is a gain rather than
a guess.

A `.jrxml` is a paginated document — that is the only thing JasperReports makes —
so `pdf` is the faithful import and carries the grouping, the page breaks, the
subtotals and the paper. The `interactive` output is the same columns and the
same charts without the pagination, and it exists because it costs nothing to
emit and it is the thing a migrating team cannot get from Jasper at all: the
report they have been mailing as a PDF, embeddable and filterable, from the
definition they already had.

Neither output invents data. Both read the columns the detail band drew, and a
block that could not be inferred is absent from both with a finding saying so.
*/

// layout is everything inferred from the bands, read once.
//
// Gathered into a value rather than recomputed per output because every one of
// these steps records findings as it goes: calling title() twice would report
// the same dropped subtitle twice, and a findings list that double-counts is one
// nobody can use to size the work.
type layout struct {
	title     string
	columns   []string
	groupBy   string
	pageBreak string
	subtotals []string
	sort      []definition.SortKey
	charts    []definition.Block
	stats     []definition.Block
}

// report assembles the report definition. A zero Report means no layout could be
// inferred, which the caller reports rather than papering over.
func (t *translation) report(ds definition.Dataset) definition.Report {
	l := t.layout()
	// Labels and formats are read from the bands and belong on the dataset's
	// fields, so this mutates the copy the report is built against.
	t.present(&ds)
	// Stats read the labels applied above, so they come after.
	l.stats = t.stats(t.captions())

	paginated := t.paginated(l)
	interactive := t.interactive(l)
	if len(paginated.Layout) == 0 && len(interactive.Layout) == 0 {
		t.found.add(Blocked, "detail",
			"no layout could be inferred: the detail band drew no field this import could read, "+
				"so there is a dataset but nothing that renders it — the query is worth keeping and the report has to be rebuilt")
		return definition.Report{}
	}

	out := definition.Report{
		Name:        ds.Name,
		Title:       l.heading(t.doc.Name),
		Description: ds.Description,
		Folder:      t.opts.Folder,
		Dataset:     ds.Name,
	}
	// Interactive first, because that is the order the examples use and the one
	// a reader opening the file expects.
	if len(interactive.Layout) > 0 {
		out.Outputs = append(out.Outputs, interactive)
	}
	if len(paginated.Layout) > 0 {
		out.Outputs = append(out.Outputs, paginated)
	}
	if err := out.Validate(); err != nil {
		// Cannot happen with the checks above, and if it does, an invalid
		// definition must not reach a file that looks like every other one.
		t.found.addf(Blocked, "report", "the inferred layout is not a valid report: %v", err)
		return definition.Report{}
	}
	return out
}

// layout reads the bands once.
func (t *translation) layout() layout {
	out := layout{title: t.title(), sort: t.sortKeys(), charts: t.chartBlocks()}
	out.columns = t.columns()
	out.groupBy, out.pageBreak, out.subtotals = t.grouping(out.columns)
	if out.groupBy != "" && !contains(out.columns, out.groupBy) {
		// Prepended: a group's column belongs at the left of the table it
		// breaks, which is where every grouped report puts it.
		out.columns = append([]string{out.groupBy}, out.columns...)
	}
	return out
}

// heading is the report's title, falling back to what Jasper called the file.
func (l layout) heading(sourceName string) string {
	if l.title != "" {
		return l.title
	}
	return sourceName
}

// paginated is the faithful import: the document Jasper actually produced.
func (t *translation) paginated(l layout) definition.Output {
	out := definition.Output{
		Name: "pdf", Renderer: definition.Paginated,
		Page:   t.page(),
		Footer: t.footer(),
	}
	if l.title != "" {
		out.Layout = append(out.Layout, definition.Block{
			Kind: definition.TextBlock, Style: "h1", Text: l.title,
		})
	}
	// Charts above the table, which is where a Jasper title or page header put
	// them; a chart in the summary band is the exception and lands here too.
	out.Layout = append(out.Layout, l.charts...)

	if len(l.columns) > 0 {
		out.Layout = append(out.Layout, definition.Block{
			Kind: definition.TableBlock, Columns: l.columns,
			Sort: l.sort, GroupBy: l.groupBy, PageBreak: l.pageBreak, Subtotals: l.subtotals,
		})
	}
	if len(out.Layout) == 1 && out.Layout[0].Kind == definition.TextBlock {
		// A heading and nothing else is a blank page with a title on it, which
		// reads as a report that found no data.
		return definition.Output{}
	}
	return out
}

// interactive is the same report without the paper.
func (t *translation) interactive(l layout) definition.Output {
	out := definition.Output{Name: "interactive", Renderer: definition.Interactive}
	out.Layout = append(out.Layout, l.stats...)
	out.Layout = append(out.Layout, l.charts...)
	if len(l.columns) > 0 {
		out.Layout = append(out.Layout, definition.Block{
			Kind: definition.TableBlock, Columns: l.columns, Sort: l.sort,
		})
	}
	return out
}

// chartBlocks collects every chart in the document, in the order the sections
// are drawn.
func (t *translation) chartBlocks() []definition.Block {
	var out []definition.Block
	for _, s := range t.allSections() {
		out = append(out, t.charts(s)...)
	}
	return out
}

// stats turns the report's own totals into tiles.
//
// A report-level variable that the original actually printed is a number the
// author chose to show, and a stat block is what shows one. Restricted to the
// ones that were drawn: a `.jrxml` accumulates variables nobody removed, and a
// tile for a total the original never displayed is a number somebody has to
// explain.
func (t *translation) stats(captions map[string]string) []definition.Block {
	drawn := t.drawnVariables()
	var out []definition.Block
	for _, v := range t.doc.Variables {
		if !drawn[v.Name] || !reportScoped(v) {
			continue
		}
		agg, ok := aggregateOf(v.Calculation)
		if !ok {
			continue
		}
		r, plain := plainRef(v.Expression)
		if !plain || r.sigil != 'F' {
			continue
		}
		name, known := t.fieldNames[r.name]
		if !known {
			continue
		}
		out = append(out, definition.Block{
			Kind: definition.StatBlock, Label: t.statLabel(v, name, captions),
			Value: definition.MeasureRef{Field: name, Aggregate: agg},
		})
	}
	return out
}

// statLabel names a tile.
//
// The caption the original printed beside the number first: "Total billed" is
// what somebody decided to call it, where the column's own heading ("Amount")
// describes the rows rather than the total of them.
func (t *translation) statLabel(v variable, field string, captions map[string]string) string {
	if caption, ok := captions[v.Name]; ok && caption != "" {
		return caption
	}
	if label := t.byName[field].Label; label != "" {
		return label
	}
	return humanise(v.Name)
}

// reportScoped reports whether a variable totals the whole report rather than a
// page or a group.
//
// Empty means Report in the schema, which is why this is not a string compare
// at the call site.
func reportScoped(v variable) bool {
	switch v.ResetType {
	case "", "Report", "None":
		return true
	}
	return false
}
