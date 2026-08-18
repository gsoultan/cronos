package jrxml

import (
	"bytes"
	"encoding/xml"
	"io"
	"sort"
)

// census walks every element in the file and reports the ones the import does
// not carry.
//
// A token scan rather than struct fields, and it is the design decision that
// makes this importer honest. The alternative — model each construct and report
// the ones you modelled — reports exactly what its author remembered, and
// JasperReports has some two hundred elements across six versions plus a
// component namespace anyone can extend. Whatever this does not recognise is
// reported as unrecognised, so the failure mode of an incomplete table is a
// noisy report rather than a silent omission.
//
// The translation still reads what it reads; this only accounts for what is
// there. Anything in handled is deliberately silent because its meaning has
// already been carried or it is pure structure.
func census(data []byte, into *findings) {
	counts := map[string]int{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charsetReader
	dec.Strict = false
	dec.Entity = xml.HTMLEntity

	for {
		tok, err := dec.Token()
		if err == io.EOF || err != nil {
			// A parse failure here cannot happen after parse() succeeded on the
			// same bytes, and if it somehow does, an incomplete census is not
			// worth failing an import that otherwise worked.
			break
		}
		if start, ok := tok.(xml.StartElement); ok {
			counts[start.Name.Local]++
		}
	}

	// Sorted so two runs over one file produce identical output. A findings
	// report that reorders itself is one nobody can diff.
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if handled[name] {
			continue
		}
		if l, known := notCarried[name]; known {
			into.addN(l.severity, l.as, l.detail, counts[name])
			continue
		}
		// The safety net. An element nobody classified is still an element that
		// did not come across.
		into.addN(Review, name,
			"a JasperReports construct this importer does not read; whatever it contributed to the report is missing",
			counts[name])
	}
}

// lost is what happened to a construct, and how much it matters.
//
// Several element names share one entry on purpose: findings merge by label and
// detail, so the eleven elements that spell out a font become one line reading
// "appearance (11)" rather than eleven lines nobody finishes.
type lost struct {
	severity Severity
	// as is the label the finding carries, which is a family rather than an
	// element when the family is what the reader needs to know about.
	as     string
	detail string
}

// handled names the elements whose meaning the translation carries, or which are
// containers whose contents are accounted for separately.
//
// Silence here is a claim: it says this element's contribution is in the emitted
// definition. Adding a name without making that true is how an importer starts
// lying.
var handled = map[string]bool{
	// Structure.
	"jasperReport": true, "queryString": true, "background": true,
	"title": true, "pageHeader": true, "columnHeader": true, "detail": true,
	"columnFooter": true, "pageFooter": true, "lastPageFooter": true,
	"summary": true, "band": true, "elementGroup": true,
	// Studio and compiler metadata, which describes the file rather than the
	// report.
	"property": true, "propertyExpression": true, "import": true,

	// Dataset shape.
	"parameter": true, "parameterDescription": true, "defaultValueExpression": true,
	"field": true, "fieldDescription": true, "sortField": true,
	"variable": true, "variableExpression": true,
	"group": true, "groupExpression": true, "groupHeader": true, "groupFooter": true,

	// Text, which is where columns and labels come from.
	"staticText": true, "text": true, "textField": true,
	"textFieldExpression": true, "reportElement": true,

	// Charts, to the extent a category and a value are a chart.
	"barChart": true, "bar3DChart": true, "stackedBarChart": true,
	"lineChart": true, "areaChart": true, "stackedAreaChart": true,
	"pieChart": true, "pie3DChart": true,
	"chart": true, "chartTitle": true, "titleExpression": true,
	"categoryDataset": true, "categorySeries": true, "categoryExpression": true,
	"valueExpression": true, "seriesExpression": true,
	"pieDataset": true, "keyExpression": true,
}

// notCarried is what to say about the constructs worth explaining.
//
// The wording is the product. Each entry says what is gone and, where there is
// one, what to do instead — because the reader is holding four hundred files and
// deciding which to open.
var notCarried = map[string]lost{
	// ---- Meaning that went missing. Someone has to look. ----
	"subreport": {Review, "subreport",
		"a subreport is a second report drawn inside this one; cronos has no nested report, so its content is missing — the usual translation is a second dataset and a second block, or a join in this dataset's query"},
	"subreportExpression":          {Review, "subreport", "a subreport is a second report drawn inside this one; cronos has no nested report, so its content is missing — the usual translation is a second dataset and a second block, or a join in this dataset's query"},
	"subreportParameter":           {Note, "subreport", "the parameters passed to a subreport went with it"},
	"subreportParameterExpression": {Note, "subreport", "the parameters passed to a subreport went with it"},
	"returnValue":                  {Review, "subreport", "a value a subreport returned to its parent is not carried; whatever used it reads nothing"},
	"subreportReturnValue":         {Review, "subreport", "a value a subreport returned to its parent is not carried; whatever used it reads nothing"},

	"crosstab": {Review, "crosstab",
		"a crosstab pivots rows into columns; cronos v1 has no pivot block, so this is not imported — a query that groups by both axes is the closest equivalent"},
	"rowGroup":                  {Note, "crosstab", "part of a crosstab that was not imported"},
	"columnGroup":               {Note, "crosstab", "part of a crosstab that was not imported"},
	"crosstabCell":              {Note, "crosstab", "part of a crosstab that was not imported"},
	"crosstabDataset":           {Note, "crosstab", "part of a crosstab that was not imported"},
	"crosstabParameter":         {Note, "crosstab", "part of a crosstab that was not imported"},
	"measure":                   {Note, "crosstab", "part of a crosstab that was not imported"},
	"measureExpression":         {Note, "crosstab", "part of a crosstab that was not imported"},
	"bucket":                    {Note, "crosstab", "part of a crosstab that was not imported"},
	"bucketExpression":          {Note, "crosstab", "part of a crosstab that was not imported"},
	"cellContents":              {Note, "crosstab", "part of a crosstab that was not imported"},
	"crosstabHeader":            {Note, "crosstab", "part of a crosstab that was not imported"},
	"crosstabRowHeader":         {Note, "crosstab", "part of a crosstab that was not imported"},
	"crosstabColumnHeader":      {Note, "crosstab", "part of a crosstab that was not imported"},
	"crosstabTotalRowHeader":    {Note, "crosstab", "part of a crosstab that was not imported"},
	"crosstabTotalColumnHeader": {Note, "crosstab", "part of a crosstab that was not imported"},
	"whenNoDataCell":            {Note, "crosstab", "part of a crosstab that was not imported"},
	"titleCell":                 {Note, "crosstab", "part of a crosstab that was not imported"},
	"headerCell":                {Note, "crosstab", "part of a crosstab that was not imported"},

	"componentElement": {Review, "component",
		"a component element — a Jasper table, list, barcode, map or spider chart — has no cronos equivalent and is not imported"},
	"genericElement":     {Review, "component", "a generic element is host-application-specific and is not imported"},
	"genericElementType": {Note, "component", "part of a generic element that was not imported"},
	"table":              {Review, "component", "a Jasper table component is not imported; a cronos table block reads columns from a dataset instead"},
	"list":               {Review, "component", "a Jasper list component is not imported"},

	// JasperReports 7 introduced a second way to spell a band's contents. A file
	// written that way parses, imports its query, and infers no layout at all —
	// which without this entry reads as "this report drew nothing".
	"element": {Review, "element",
		`this file uses the JasperReports 7 element syntax — <element kind="textField"> rather than <textField> inside a band — which this importer does not read, so no layout came across; the query did, and the layout has to be rebuilt`},
	"part": {Review, "part",
		"a JasperReports Book part is a whole report bound into a sequence; cronos has no book, so each part is imported on its own or not at all"},

	"image": {Review, "image",
		"an image is not carried; a logo or letterhead belongs in the paginated output's header template, which is a Typst file"},
	"imageExpression": {Note, "image", "the expression naming an image that was not carried"},

	"scriptlet": {Review, "scriptlet",
		"a scriptlet is Java that runs during the report; there is no code execution in a cronos definition, so anything it computed is missing — it usually becomes SQL"},
	"scriptletParameter": {Note, "scriptlet", "a parameter passed to a scriptlet that was not carried"},

	"filterExpression": {Review, "filterExpression",
		"rows were dropped by an expression after the query returned; cronos filters in SQL, so this belongs in the dataset's WHERE clause or its rowLevelSecurity"},

	"printWhenExpression": {Review, "printWhenExpression",
		"an element was drawn conditionally; cronos has no conditional block, so it is imported unconditionally and may show what the original hid"},

	"subDataset": {Review, "subDataset",
		"a second query inside the file, feeding a chart or crosstab; cronos would express it as its own Dataset and a block that reads it, which this import does not create"},
	"datasetRun":                 {Note, "subDataset", "the binding between an element and a second query, which was not imported"},
	"datasetParameter":           {Note, "subDataset", "the binding between an element and a second query, which was not imported"},
	"datasetParameterExpression": {Note, "subDataset", "the binding between an element and a second query, which was not imported"},
	// <dataset> is both the binding to a subDataset and the per-chart reset and
	// increment settings, so the wording has to be true of either.
	"dataset": {Note, "subDataset",
		"an element's own dataset settings — when it resets, when it increments, or a second query it reads — are not carried"},
	"connectionExpression": {Note, "subDataset", "a subreport or chart's own connection, which is a cronos DataSource instead"},
	"dataSourceExpression": {Note, "subDataset", "a subreport or chart's own data source, which is a cronos DataSource instead"},

	"initialValueExpression": {Note, "variable",
		"a variable's initial value is not carried; a cronos subtotal starts at zero"},

	"anchorNameExpression":         {Note, "hyperlink", "bookmarks and links are not carried"},
	"hyperlinkReferenceExpression": {Note, "hyperlink", "bookmarks and links are not carried"},
	"hyperlinkAnchorExpression":    {Note, "hyperlink", "bookmarks and links are not carried"},
	"hyperlinkPageExpression":      {Note, "hyperlink", "bookmarks and links are not carried"},
	"hyperlinkTooltipExpression":   {Note, "hyperlink", "bookmarks and links are not carried"},
	"hyperlinkParameter":           {Note, "hyperlink", "bookmarks and links are not carried"},
	"hyperlinkParameterExpression": {Note, "hyperlink", "bookmarks and links are not carried"},

	// ---- Charts whose shape has no cronos equivalent. ----
	"xyDataset":         {Review, "chart", "an XY, time-series or scatter chart plots two measures against each other; cronos charts a dimension against a measure, so this is not imported"},
	"xySeries":          {Note, "chart", "part of an XY chart that was not imported"},
	"xExpression":       {Note, "chart", "part of an XY chart that was not imported"},
	"yExpression":       {Note, "chart", "part of an XY chart that was not imported"},
	"timeSeriesDataset": {Review, "chart", "an XY, time-series or scatter chart plots two measures against each other; cronos charts a dimension against a measure, so this is not imported"},
	"timeSeries":        {Note, "chart", "part of a time-series chart that was not imported"},
	"scatterChart":      {Review, "chart", "an XY, time-series or scatter chart plots two measures against each other; cronos charts a dimension against a measure, so this is not imported"},
	"bubbleChart":       {Review, "chart", "a bubble chart has a third axis cronos does not draw; it is not imported"},
	"meterChart":        {Review, "chart", "a meter or thermometer chart draws one value against a range; the nearest cronos block is a stat, which this import does not infer from it"},
	"thermometerChart":  {Review, "chart", "a meter or thermometer chart draws one value against a range; the nearest cronos block is a stat, which this import does not infer from it"},
	"multiAxisChart":    {Review, "chart", "a multi-axis chart draws several measures on separate scales; cronos charts one measure, so this is not imported"},
	"ganttChart":        {Review, "chart", "a Gantt chart is not a cronos chart kind and is not imported"},
	"candlestickChart":  {Review, "chart", "a candlestick chart is not a cronos chart kind and is not imported"},
	"highLowChart":      {Review, "chart", "a high-low chart is not a cronos chart kind and is not imported"},
	"spiderChart":       {Review, "chart", "a spider chart is not a cronos chart kind and is not imported"},

	// ---- Appearance. Expected, and expected to be ignored. ----
	"textElement":       {Note, "appearance", appearanceDetail},
	"font":              {Note, "appearance", appearanceDetail},
	"reportFont":        {Note, "appearance", appearanceDetail},
	"box":               {Note, "appearance", appearanceDetail},
	"pen":               {Note, "appearance", appearanceDetail},
	"topPen":            {Note, "appearance", appearanceDetail},
	"leftPen":           {Note, "appearance", appearanceDetail},
	"bottomPen":         {Note, "appearance", appearanceDetail},
	"rightPen":          {Note, "appearance", appearanceDetail},
	"paragraph":         {Note, "appearance", appearanceDetail},
	"style":             {Note, "appearance", appearanceDetail},
	"conditionalStyle":  {Note, "appearance", appearanceDetail},
	"styleExpression":   {Note, "appearance", appearanceDetail},
	"template":          {Note, "appearance", appearanceDetail},
	"graphicElement":    {Note, "appearance", appearanceDetail},
	"line":              {Note, "appearance", appearanceDetail},
	"rectangle":         {Note, "appearance", appearanceDetail},
	"ellipse":           {Note, "appearance", appearanceDetail},
	"frame":             {Note, "appearance", appearanceDetail},
	"break":             {Note, "appearance", "an explicit page break inside a band; cronos breaks per group or per page"},
	"patternExpression": {Note, "appearance", "a computed number or date format; cronos formats by the field's declared type and format"},

	// Chart appearance, kept as its own family: a reader who does not care about
	// fonts may still want to know the axis titles and colours are gone.
	"plot":                {Note, "chart appearance", chartAppearance},
	"barPlot":             {Note, "chart appearance", chartAppearance},
	"bar3DPlot":           {Note, "chart appearance", chartAppearance},
	"stackedBarPlot":      {Note, "chart appearance", chartAppearance},
	"linePlot":            {Note, "chart appearance", chartAppearance},
	"areaPlot":            {Note, "chart appearance", chartAppearance},
	"stackedAreaPlot":     {Note, "chart appearance", chartAppearance},
	"piePlot":             {Note, "chart appearance", chartAppearance},
	"pie3DPlot":           {Note, "chart appearance", chartAppearance},
	"itemLabel":           {Note, "chart appearance", chartAppearance},
	"valueAxisFormat":     {Note, "chart appearance", chartAppearance},
	"categoryAxisFormat":  {Note, "chart appearance", chartAppearance},
	"timeAxisFormat":      {Note, "chart appearance", chartAppearance},
	"axisFormat":          {Note, "chart appearance", chartAppearance},
	"axisLabelExpression": {Note, "chart appearance", chartAppearance},
	"chartLegend":         {Note, "chart appearance", chartAppearance},
	"chartSubtitle":       {Note, "chart appearance", chartAppearance},
	"subtitleExpression":  {Note, "chart appearance", chartAppearance},
	"seriesColor":         {Note, "chart appearance", chartAppearance},
	"labelExpression":     {Note, "chart appearance", "a chart's per-point labels are not carried"},
	"otherKeyExpression":  {Note, "chart appearance", chartAppearance},
	"valueDisplay":        {Note, "chart appearance", chartAppearance},
	"dataRange":           {Note, "chart appearance", chartAppearance},
	"lowExpression":       {Note, "chart appearance", chartAppearance},
	"highExpression":      {Note, "chart appearance", chartAppearance},
}

const appearanceDetail = "fonts, colours, borders, rules and pixel positions are not carried; cronos styles by theme and lays out by block"

const chartAppearance = "a chart's axis titles, colours and value formatting are not carried"
