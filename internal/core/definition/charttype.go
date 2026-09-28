package definition

// ChartType is which visualisation a chart block draws.
//
// This is deliberately not a BlockKind — see blockkind.go. Adding a chart type
// here costs every renderer one case in one switch; adding a block kind costs
// them a new concept each.
//
// The type was a free string until this file existed, which meant the builder
// could emit `chart: line` for months while the viewer answered "line charts
// need a newer viewer", and nothing in between said so. A closed set turns that
// into a validation error at the point somebody writes it.
type ChartType string

const (
	BarChart ChartType = "bar"
	// ColumnChart is the bar chart stood up, for categories read left to
	// right: months, weeks, the steps of something.
	ColumnChart  ChartType = "column"
	LineChart    ChartType = "line"
	AreaChart    ChartType = "area"
	PieChart     ChartType = "pie"
	DonutChart   ChartType = "donut"
	ScatterChart ChartType = "scatter"
	BubbleChart  ChartType = "bubble"
	MapChart     ChartType = "map"

	ComboChart     ChartType = "combo"
	FunnelChart    ChartType = "funnel"
	WaterfallChart ChartType = "waterfall"
	HeatmapChart   ChartType = "heatmap"
	GaugeChart     ChartType = "gauge"
	TreemapChart   ChartType = "treemap"

	// RadarChart draws each category as a spoke and each series as a shape
	// through its values: a profile across a handful of measures.
	RadarChart ChartType = "radar"
	// BulletChart is a gauge's reading in a row's height, one per category:
	// a value, the target it is read against, and shades of how near.
	BulletChart ChartType = "bullet"
	// HistogramChart cuts a number's range into bins and counts the rows in
	// each: how a measure spreads, rather than what each category sums to.
	HistogramChart ChartType = "histogram"
	// BoxplotChart draws each category's spread: the middle half of its rows
	// as a box, its median across it, and whiskers to the rest.
	BoxplotChart ChartType = "boxplot"
	// SankeyChart draws how a measure flows from each category of x to each
	// of series: a band per pair, as thick as its share of the whole.
	SankeyChart ChartType = "sankey"
	// SunburstChart draws parts within parts as rings: series around the
	// middle, and each one's categories of x around it.
	SunburstChart ChartType = "sunburst"
	// CalendarChart draws a measure per day of a date, a year a row of weeks:
	// the shape of a working week and a season, which a line of the same
	// days smooths away.
	CalendarChart ChartType = "calendar"
)

// chartTypes is every type, in the order an error message should list them.
var chartTypes = []ChartType{
	BarChart, ColumnChart, LineChart, AreaChart, PieChart, DonutChart,
	ScatterChart, BubbleChart, MapChart,
	ComboChart, FunnelChart, WaterfallChart, HeatmapChart, GaugeChart, TreemapChart,
	RadarChart, BulletChart, HistogramChart, BoxplotChart, SankeyChart, SunburstChart,
	CalendarChart,
}

// Valid reports whether c is a type every renderer knows how to refuse or draw.
func (c ChartType) Valid() bool {
	for _, k := range chartTypes {
		if c == k {
			return true
		}
	}
	return false
}

// Categorical reports whether the chart buckets a dimension and folds a
// measure — one label, one number, which is what `x` and `y` mean for it.
func (c ChartType) Categorical() bool {
	switch c {
	case BarChart, ColumnChart, LineChart, AreaChart, PieChart, DonutChart,
		WaterfallChart, TreemapChart, HeatmapChart, RadarChart, SankeyChart, SunburstChart,
		CalendarChart:
		return true
	}
	return false
}

// Mixes reports whether the chart draws several measures together, each with
// its own mark — which is what a combo chart is.
func (c ChartType) Mixes() bool { return c == ComboChart }

// Metered reports whether the chart reads a list of measures rather than one.
//
// A combo always does. A funnel does when its stages are separate columns,
// which is how most warehouses model one — and does not when they are rows of
// a stage dimension, which is the other honest shape. Both are supported
// because both are what the data already looks like.
func (c ChartType) Metered() bool { return c == ComboChart || c == FunnelChart }

// Gridded reports whether the chart needs two dimensions to place a value.
func (c ChartType) Gridded() bool { return c == HeatmapChart }

// Paired reports whether series is the chart's second dimension rather than
// an optional split — what each flow goes to, which ring a part sits in — so
// a chart of this type without one is refused rather than drawn as half of it.
func (c ChartType) Paired() bool {
	return c == HeatmapChart || c == SankeyChart || c == SunburstChart
}

// Folded reports whether the chart reads one number for the whole set, with no
// bucketing at all.
func (c ChartType) Folded() bool { return c == GaugeChart }

// Targeted reports whether the chart reads its values against a target: a
// gauge's one number, or each of a bullet chart's.
func (c ChartType) Targeted() bool { return c == GaugeChart || c == BulletChart }

// Ordinal reports whether the chart's categories have an order that carries
// meaning, so they take a one-hue ramp rather than eight identities.
//
// Swapping two funnel stages changes what the chart says; swapping two regions
// on a bar chart does not. That difference is the whole reason the ramp exists
// — a reader should see the sequence in the colour rather than have to read
// the labels left to right to discover there was one.
func (c ChartType) Ordinal() bool { return c == FunnelChart || c == TreemapChart }

// Diverging reports whether the chart's marks have a sign, so they take two
// hues either side of a neutral rather than one.
func (c ChartType) Diverging() bool { return c == WaterfallChart }

// Plots reports whether both axes are measures.
//
// Scatter is the chart type that broke the assumption baked into chartSQL:
// `x` is a dimension to GROUP BY everywhere else, and here it is a number to
// read per row. Asking the type rather than the field's role keeps that
// difference in one place.
func (c ChartType) Plots() bool { return c == ScatterChart || c == BubbleChart }

// Distributes reports whether the chart draws how a number's rows spread,
// read row by row in the database rather than folded per category: a
// histogram's bins, a box plot's quartiles.
func (c ChartType) Distributes() bool { return c == HistogramChart || c == BoxplotChart }

// Divides reports whether the chart shows parts of a whole, so the whole is
// worth saying: a donut carries it in its middle.
func (c ChartType) Divides() bool { return c == PieChart || c == DonutChart }

// Geographic reports whether the chart reads coordinates rather than an axis.
func (c ChartType) Geographic() bool { return c == MapChart }

// Stacks reports whether a series dimension stacks rather than drawing beside.
//
// Pie and donut are already part-to-whole, so a series dimension on them would
// be a second whole with nowhere to go; line and scatter stack into a shape
// that reads as a total nobody measured.
func (c ChartType) Stacks() bool { return c == BarChart || c == ColumnChart || c == AreaChart }

// MultiSeries reports whether the type can draw more than one series at once.
func (c ChartType) MultiSeries() bool {
	switch c {
	case BarChart, ColumnChart, LineChart, AreaChart, ScatterChart, BubbleChart, RadarChart:
		return true
	// A heatmap's second dimension is not an alternative to one series, it is
	// the other axis of the grid — so `series` is required rather than
	// optional, as it is for where a sankey's flows go and which ring a
	// sunburst's parts sit in. A treemap nests with it: the outer rectangles
	// are the series and the inner ones are x.
	case HeatmapChart, TreemapChart, SankeyChart, SunburstChart:
		return true
	// A map colours its points by category, on the layers that have not
	// already given colour to the value — which Block.validateMapSeries
	// checks, because it depends on the layers rather than on the type.
	case MapChart:
		return true
	}
	return false
}

// ChartTypeNames lists every valid type, for an error message or a form.
func ChartTypeNames() []string {
	out := make([]string, len(chartTypes))
	for i, c := range chartTypes {
		out[i] = string(c)
	}
	return out
}
