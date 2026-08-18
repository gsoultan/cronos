package jrxml

import (
	"github.com/gsoultan/cronos/internal/core/definition"
)

// charts translates the chart elements in a section into chart blocks.
//
// Jasper draws a chart from a category and a value expression, which is the same
// pair a cronos chart block takes — so unlike the table, this is a translation
// rather than an inference. What does not survive is everything around it: the
// plot, the axes, the colours, and any chart whose shape is two measures against
// each other rather than a dimension against one. Those are reported by the
// census; this reports the ones it started to translate and could not finish.
func (t *translation) charts(in section) []definition.Block {
	var out []definition.Block
	for _, c := range in.charts() {
		if blk, ok := t.chart(c); ok {
			out = append(out, blk)
		}
	}
	return out
}

func (t *translation) chart(c placedChart) (definition.Block, bool) {
	if sub := c.chart.subDataset(); sub != "" {
		t.found.addf(Review, c.from,
			"a chart reads the second query %q rather than the report's own; cronos would express that as its own Dataset and a block bound to it, which this import does not create",
			sub)
		return definition.Block{}, false
	}

	x, y, series, ok := t.chartAxes(c)
	if !ok {
		return definition.Block{}, false
	}
	if c.from == "pieChart" || c.from == "pie3DChart" {
		// Converted rather than dropped. The binding is the valuable part and it
		// is identical; the shape is a choice the author can remake in the
		// builder, and a missing chart is harder to notice than a changed one.
		t.found.addf(Review, c.from,
			"cronos has no pie chart, so %q was imported as a bar chart of the same values",
			t.chartTitle(c))
		x.Grain = ""
	}

	blk := definition.Block{
		Kind: definition.ChartBlock, Chart: c.kind,
		Title: t.chartTitle(c), X: x, Y: y, Series: series,
	}
	return blk, true
}

// chartAxes resolves the category and value expressions to fields.
func (t *translation) chartAxes(c placedChart) (x definition.DimensionRef, y definition.MeasureRef,
	series definition.DimensionRef, ok bool) {

	category, value, seriesExpr := c.chart.bindings()
	if category == "" || value == "" {
		t.found.addf(Review, c.from,
			"a chart declares no category or no value expression, so there is nothing to plot")
		return x, y, series, false
	}

	xName, found := t.chartField(c.from, category, "category")
	if !found {
		return x, y, series, false
	}
	yName, found := t.chartField(c.from, value, "value")
	if !found {
		return x, y, series, false
	}

	x = definition.DimensionRef{Field: xName}
	if t.byName[xName].Type == "date" {
		// Faithful: Jasper grouped by whatever the expression returned, and for
		// a raw date that is one category per timestamp. Said out loud because
		// it is a chart with two thousand bars, and the fix is one word.
		t.found.addf(Note, c.from,
			"the chart's x axis is the date %q with no bucketing, which draws one category per distinct value — set grain: month on the block if it should bucket",
			xName)
	}

	y = definition.MeasureRef{Field: yName}
	if t.byName[yName].Role != definition.Measure {
		// The block has to say how to fold it, because the field does not.
		y.Aggregate = "sum"
		t.found.addf(Review, c.from,
			"the chart plots %q, which imported as a dimension rather than a measure; the block sums it — check that is what the chart showed",
			yName)
	}

	if seriesExpr != "" {
		if name, isField := t.chartField(c.from, seriesExpr, "series"); isField {
			series = definition.DimensionRef{Field: name}
		}
		// A literal series name is the common case — one series, named — and
		// needs nothing: a cronos chart with no series field draws exactly one.
	}
	return x, y, series, true
}

// chartField resolves one chart expression to a field of the dataset.
func (t *translation) chartField(from, expr, role string) (string, bool) {
	r, plain := plainRef(expr)
	if !plain || r.sigil != 'F' {
		if _, isLiteral := literalString(expr); isLiteral && role == "series" {
			return "", false
		}
		t.found.addf(Review, from,
			"a chart's %s is the Java expression %s; cronos charts a field, so the chart is not imported",
			role, oneLine(expr))
		return "", false
	}
	name, known := t.fieldNames[r.name]
	if !known {
		t.found.addf(Review, from,
			"a chart's %s reads $F{%s}, which the report does not declare as a field", role, r.name)
		return "", false
	}
	return name, true
}

func (t *translation) chartTitle(c placedChart) string {
	if title, ok := literalString(c.chart.Chart.Title.Expression); ok {
		return collapse(title)
	}
	return ""
}

// bindings returns the category, value and series expressions, whichever dataset
// shape the chart uses.
func (c chartElement) bindings() (category, value, series string) {
	if len(c.Category.Series) > 0 {
		s := c.Category.Series[0]
		return s.Category, s.Value, s.Series
	}
	// A pie chart keys rather than categorises, and has no series.
	return c.Pie.Key, c.Pie.Value, ""
}

// subDataset names the second query a chart runs against, if it has one.
func (c chartElement) subDataset() string {
	for _, name := range []string{
		c.Chart.Dataset.SubDataset, c.Category.Run.SubDataset, c.Pie.Run.SubDataset,
	} {
		if name != "" {
			return name
		}
	}
	return ""
}
