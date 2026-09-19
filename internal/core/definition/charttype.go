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
	BarChart     ChartType = "bar"
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
)

// chartTypes is every type, in the order an error message should list them.
var chartTypes = []ChartType{
	BarChart, LineChart, AreaChart, PieChart, DonutChart,
	ScatterChart, BubbleChart, MapChart,
	ComboChart, FunnelChart, WaterfallChart, HeatmapChart, GaugeChart, TreemapChart,
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
	case BarChart, LineChart, AreaChart, PieChart, DonutChart,
		WaterfallChart, TreemapChart, HeatmapChart:
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

// Folded reports whether the chart reads one number for the whole set, with no
// bucketing at all.
func (c ChartType) Folded() bool { return c == GaugeChart }

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

// Geographic reports whether the chart reads coordinates rather than an axis.
func (c ChartType) Geographic() bool { return c == MapChart }

// Stacks reports whether a series dimension stacks rather than drawing beside.
//
// Pie and donut are already part-to-whole, so a series dimension on them would
// be a second whole with nowhere to go; line and scatter stack into a shape
// that reads as a total nobody measured.
func (c ChartType) Stacks() bool { return c == BarChart || c == AreaChart }

// MultiSeries reports whether the type can draw more than one series at once.
func (c ChartType) MultiSeries() bool {
	switch c {
	case BarChart, LineChart, AreaChart, ScatterChart, BubbleChart:
		return true
	// A heatmap's second dimension is not an alternative to one series, it is
	// the other axis of the grid — so `series` is required rather than
	// optional. A treemap nests with it: the outer rectangles are the series
	// and the inner ones are x.
	case HeatmapChart, TreemapChart:
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
