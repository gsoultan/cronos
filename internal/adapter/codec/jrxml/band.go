package jrxml

// band is one horizontal strip of a Jasper report.
//
// The unit of a band-based layout: it is drawn once per whatever owns it — once
// for a title, once per record for a detail, once per group for a group header —
// and the elements inside it are positioned absolutely within it.
type band struct {
	Height int `xml:"height,attr"`
	// SplitType governs what happens when a band meets a page boundary. There
	// is no cronos equivalent; a table block breaks between rows.
	SplitType string `xml:"splitType,attr"`
	contents
}

// contents is the set of elements a band or a frame may hold.
//
// Embedded rather than repeated, and recursive through frame, because a frame is
// a box drawn around elements and authors nest them freely — a text field three
// frames deep is still the column it looks like, and an importer that only
// looked at the top level would import a table with no columns.
type contents struct {
	StaticTexts []staticText `xml:"staticText"`
	TextFields  []textField  `xml:"textField"`
	Frames      []frame      `xml:"frame"`
	// Subreports are the largest single thing that does not survive. Counted
	// here so the finding can name how many and where.
	Subreports []subreport `xml:"subreport"`

	// Charts, one field per element name because that is how the schema spells
	// them. Interleaving order is lost and does not matter: elements carry
	// coordinates, and the layout is rebuilt from those rather than from the
	// order the file happened to list them in.
	BarCharts        []chartElement `xml:"barChart"`
	Bar3DCharts      []chartElement `xml:"bar3DChart"`
	StackedBarCharts []chartElement `xml:"stackedBarChart"`
	LineCharts       []chartElement `xml:"lineChart"`
	AreaCharts       []chartElement `xml:"areaChart"`
	StackedAreaChart []chartElement `xml:"stackedAreaChart"`
	PieCharts        []chartElement `xml:"pieChart"`
	Pie3DCharts      []chartElement `xml:"pie3DChart"`
}

// frame is a box around elements, with its own position.
//
// Its children's coordinates are relative to it, which is why offset exists:
// flattening a frame without adding its origin puts every column at x=0 and the
// inferred table comes out in file order rather than page order.
type frame struct {
	Element reportElement `xml:"reportElement"`
	contents
}

// reportElement is where and how big, plus the two things about it that carry
// meaning: the style it names and whether it is drawn at all.
type reportElement struct {
	X      int    `xml:"x,attr"`
	Y      int    `xml:"y,attr"`
	Width  int    `xml:"width,attr"`
	Height int    `xml:"height,attr"`
	Style  string `xml:"style,attr"`
	// PrintWhenExpression makes an element conditional. cronos has no
	// conditional block, so an element carrying one is reported: importing it
	// unconditionally would show something the original hid.
	PrintWhenExpression string `xml:"printWhenExpression"`
}

// staticText is a literal on the page: a column heading, a label, a title.
type staticText struct {
	Element reportElement `xml:"reportElement"`
	Text    string        `xml:"text"`
}

// textField is a value on the page. Its expression is the interesting part and
// usually the whole difficulty.
type textField struct {
	Element reportElement `xml:"reportElement"`
	// Pattern is a Java format string. It informs the field's format but is not
	// carried literally — cronos formats by field semantics, not per element.
	Pattern string `xml:"pattern,attr"`
	// EvaluationTime deferred to Report or Group is how Jasper prints a total
	// before the rows it totals. Worth noticing on a detail field, where it
	// means the column is not what it appears to be.
	EvaluationTime string `xml:"evaluationTime,attr"`
	Expression     string `xml:"textFieldExpression"`
}

// subreport is another report drawn inside this one.
type subreport struct {
	Element reportElement `xml:"reportElement"`
	// Expression is a Java expression producing a path or a compiled report,
	// so the file it names is often not literally in there.
	Expression string `xml:"subreportExpression"`
}

// chartElement is any of Jasper's chart elements. They differ in their plot and
// their dataset shape; the parts an import needs are the same.
type chartElement struct {
	Chart chartFrame `xml:"chart"`
	// Category is the dataset shape behind bar, line and area charts.
	Category categoryDataset `xml:"categoryDataset"`
	// Pie is the shape behind pie charts: a key rather than a category.
	Pie pieDataset `xml:"pieDataset"`
}

type chartFrame struct {
	Element reportElement `xml:"reportElement"`
	Title   chartTitle    `xml:"chartTitle"`
	// Dataset names a subDataset when the chart does not read the main query.
	Dataset chartDatasetRun `xml:"dataset"`
}

type chartTitle struct {
	Expression string `xml:"titleExpression"`
}

// chartDatasetRun is the dataset a chart runs against. A subDataset reference
// here means the chart reads a query of its own, which the import cannot follow
// into a single cronos block.
type chartDatasetRun struct {
	SubDataset string `xml:"subDataset,attr"`
}

type categoryDataset struct {
	Series []categorySeries `xml:"categorySeries"`
	Run    chartDatasetRun  `xml:"dataset"`
}

type categorySeries struct {
	// Series names the line or bar group. Often a literal string, in which case
	// there is only one series and cronos needs no series field.
	Series string `xml:"seriesExpression"`
	// Category is the x axis.
	Category string `xml:"categoryExpression"`
	// Value is the y axis.
	Value string `xml:"valueExpression"`
}

type pieDataset struct {
	Key   string          `xml:"keyExpression"`
	Value string          `xml:"valueExpression"`
	Run   chartDatasetRun `xml:"dataset"`
}

// textFieldsAt flattens every text field in the band, with frame origins added.
func (c contents) textFieldsAt(dx, dy int) []placedField {
	out := make([]placedField, 0, len(c.TextFields))
	for _, tf := range c.TextFields {
		out = append(out, placedField{field: tf, x: tf.Element.X + dx, y: tf.Element.Y + dy})
	}
	for _, f := range c.Frames {
		out = append(out, f.textFieldsAt(dx+f.Element.X, dy+f.Element.Y)...)
	}
	return out
}

// staticTextsAt flattens every static text in the band, with frame origins
// added.
func (c contents) staticTextsAt(dx, dy int) []placedText {
	out := make([]placedText, 0, len(c.StaticTexts))
	for _, st := range c.StaticTexts {
		out = append(out, placedText{text: st, x: st.Element.X + dx, y: st.Element.Y + dy})
	}
	for _, f := range c.Frames {
		out = append(out, f.staticTextsAt(dx+f.Element.X, dy+f.Element.Y)...)
	}
	return out
}

// charts flattens every chart in the band, tagged with the cronos chart kind it
// maps to and the Jasper element it came from.
func (c contents) charts() []placedChart {
	var out []placedChart
	add := func(kind, from string, found []chartElement) {
		for _, ch := range found {
			out = append(out, placedChart{chart: ch, kind: kind, from: from})
		}
	}
	add("bar", "barChart", c.BarCharts)
	add("bar", "bar3DChart", c.Bar3DCharts)
	add("bar", "stackedBarChart", c.StackedBarCharts)
	add("line", "lineChart", c.LineCharts)
	add("area", "areaChart", c.AreaCharts)
	add("area", "stackedAreaChart", c.StackedAreaChart)
	add("bar", "pieChart", c.PieCharts)
	add("bar", "pie3DChart", c.Pie3DCharts)
	for _, f := range c.Frames {
		out = append(out, f.charts()...)
	}
	return out
}

// subreportsIn counts subreports at any depth.
func (c contents) subreportsIn() []subreport {
	out := append([]subreport(nil), c.Subreports...)
	for _, f := range c.Frames {
		out = append(out, f.subreportsIn()...)
	}
	return out
}

// placedField is a text field with its position resolved through any frames.
type placedField struct {
	field textField
	x, y  int
}

// placedText is a static text with its position resolved through any frames.
type placedText struct {
	text staticText
	x, y int
}

// placedChart is a chart with the cronos kind it becomes and the element name it
// came from, which the finding needs in order to say what it changed.
type placedChart struct {
	chart chartElement
	kind  string
	from  string
}

// fields flattens every text field across a section's bands.
func (s section) fields() []placedField {
	var out []placedField
	for _, b := range s.Bands {
		out = append(out, b.textFieldsAt(0, 0)...)
	}
	return out
}

// texts flattens every static text across a section's bands.
func (s section) texts() []placedText {
	var out []placedText
	for _, b := range s.Bands {
		out = append(out, b.staticTextsAt(0, 0)...)
	}
	return out
}

// charts flattens every chart across a section's bands.
func (s section) charts() []placedChart {
	var out []placedChart
	for _, b := range s.Bands {
		out = append(out, b.charts()...)
	}
	return out
}

// empty reports whether the section draws nothing.
//
// A band with a height and no elements is empty for import purposes: it is
// whitespace, and whitespace is appearance.
func (s section) empty() bool {
	for _, b := range s.Bands {
		if len(b.textFieldsAt(0, 0)) > 0 || len(b.staticTextsAt(0, 0)) > 0 ||
			len(b.charts()) > 0 || len(b.subreportsIn()) > 0 {
			return false
		}
	}
	return true
}
