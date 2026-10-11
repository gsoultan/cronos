package jrxml

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// crosstab is a Jasper crosstab: groups down the side and across the top, and
// the measures folded where they meet. A cronos pivot is the same thing with
// one group each way.
type crosstab struct {
	Element reportElement `xml:"reportElement"`
	// Run names a subDataset when the crosstab does not read the main query.
	Run          chartDatasetRun   `xml:"crosstabDataset>dataset>datasetRun"`
	RowGroups    []crosstabGroup   `xml:"rowGroup"`
	ColumnGroups []crosstabGroup   `xml:"columnGroup"`
	Measures     []crosstabMeasure `xml:"measure"`
}

type crosstabGroup struct {
	Name   string `xml:"name,attr"`
	Bucket string `xml:"bucket>bucketExpression"`
}

type crosstabMeasure struct {
	Name        string `xml:"name,attr"`
	Calculation string `xml:"calculation,attr"`
	Expression  string `xml:"measureExpression"`
}

// crosstabs flattens every crosstab in the band, frames included.
func (c contents) crosstabs() []crosstab {
	out := append([]crosstab(nil), c.Crosstabs...)
	for _, f := range c.Frames {
		out = append(out, f.crosstabs()...)
	}
	return out
}

// pivotBlocks translates every crosstab in the document, in the order the
// sections are drawn: a pivot per measure, since a pivot folds one.
func (t *translation) pivotBlocks() []definition.Block {
	var out []definition.Block
	for _, s := range t.allSections() {
		for _, b := range s.Bands {
			for _, c := range b.crosstabs() {
				out = append(out, t.pivots(c)...)
			}
		}
	}
	return out
}

func (t *translation) pivots(c crosstab) []definition.Block {
	if c.Run.SubDataset != "" {
		t.found.addf(Review, "crosstab",
			"a crosstab reads the second query %q rather than the report's own; cronos would express that as its own Dataset and a pivot block bound to it, which this import does not create",
			c.Run.SubDataset)
		return nil
	}
	if len(c.RowGroups) != 1 || len(c.ColumnGroups) != 1 {
		t.found.addf(Review, "crosstab",
			"a crosstab with %d row groups and %d column groups; a pivot crosses one of each, so it is not imported — a pivot per pair, or a dimension the query concatenates, is the closest equivalent",
			len(c.RowGroups), len(c.ColumnGroups))
		return nil
	}
	rows, ok := t.crosstabField(c.RowGroups[0].Bucket, "row group")
	if !ok {
		return nil
	}
	columns, ok := t.crosstabField(c.ColumnGroups[0].Bucket, "column group")
	if !ok {
		return nil
	}
	var out []definition.Block
	for _, m := range c.Measures {
		if blk, ok := t.pivot(rows, columns, m); ok {
			out = append(out, blk)
		}
	}
	return out
}

// pivot translates one measure of a crosstab whose groups have been resolved.
func (t *translation) pivot(rows, columns definition.DimensionRef, m crosstabMeasure) (definition.Block, bool) {
	agg, ok := aggregateOf(m.Calculation)
	if !ok {
		t.found.addf(Review, "crosstab",
			"a crosstab measure folds by %q, which cronos has no equivalent of — sum, count, average, lowest and highest are", m.Calculation)
		return definition.Block{}, false
	}
	value, ok := t.crosstabField(m.Expression, "measure")
	if !ok {
		return definition.Block{}, false
	}
	return definition.Block{
		Kind:  definition.PivotBlock,
		Title: fmt.Sprintf("%s by %s and %s", humanise(value.Field), humanise(rows.Field), humanise(columns.Field)),
		Value: definition.MeasureRef{Field: value.Field, Aggregate: agg},
		Pivot: &definition.Pivot{Rows: rows, Columns: columns},
	}, true
}

// crosstabField resolves one crosstab expression to a field of the dataset.
func (t *translation) crosstabField(expr, role string) (definition.DimensionRef, bool) {
	r, plain := plainRef(expr)
	if !plain || r.sigil != 'F' {
		t.found.addf(Review, "crosstab",
			"a crosstab's %s is the Java expression %s; a pivot crosses fields, so it is not imported", role, oneLine(expr))
		return definition.DimensionRef{}, false
	}
	name, known := t.fieldNames[r.name]
	if !known {
		t.found.addf(Review, "crosstab",
			"a crosstab's %s reads $F{%s}, which the report does not declare as a field", role, r.name)
		return definition.DimensionRef{}, false
	}
	if role != "measure" && t.byName[name].Type == "date" {
		t.found.addf(Note, "crosstab",
			"the crosstab's %s is the date %q with no bucketing, which makes a line per distinct day — set grain: month on the pivot if it should bucket",
			role, name)
	}
	return definition.DimensionRef{Field: name}, true
}
