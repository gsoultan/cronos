package definition

import "fmt"

// Validate reports every reason the report cannot be stored.
//
// It cannot check that a block's field exists — that needs the datasets, which
// this package deliberately does not reach for. query.CheckReport does it
// where both are in hand.
func (r Report) Validate() error {
	if !slug.MatchString(r.Name) {
		return fmt.Errorf("%w: name %q must be lowercase letters, digits and dashes", ErrInvalid, r.Name)
	}
	if len(r.Outputs) == 0 {
		return fmt.Errorf("%w: report %q has no outputs", ErrInvalid, r.Name)
	}
	for _, f := range r.Filters {
		if err := f.Validate(); err != nil {
			return err
		}
	}
	for param := range r.Params {
		if _, ok := r.paramNamed(param); !ok {
			return fmt.Errorf("%w: report %q overrides param %q, which is not an identifier",
				ErrInvalid, r.Name, param)
		}
	}
	return r.validateOutputs()
}

func (r Report) paramNamed(name string) (string, bool) {
	return name, identifier.MatchString(name)
}

func (r Report) validateOutputs() error {
	seen := map[string]bool{}
	for _, o := range r.Outputs {
		switch {
		case o.Name == "":
			return fmt.Errorf("%w: report %q has an output with no name", ErrInvalid, r.Name)
		case seen[o.Name]:
			return fmt.Errorf("%w: report %q has two outputs called %q", ErrInvalid, r.Name, o.Name)
		case !o.Renderer.Valid():
			return fmt.Errorf("%w: output %q has renderer %q, want interactive, paginated or spreadsheet",
				ErrInvalid, o.Name, o.Renderer)
		}
		seen[o.Name] = true
		if err := o.validate(r.Dataset); err != nil {
			return err
		}
	}
	return nil
}

func (o Output) validate(reportDefault string) error {
	if o.Renderer == Spreadsheet {
		if len(o.Sheets) == 0 {
			return fmt.Errorf("%w: spreadsheet output %q has no sheets", ErrInvalid, o.Name)
		}
		return nil
	}
	if len(o.Layout) == 0 {
		// An output with no layout renders an empty document, which looks like
		// a report that found no data rather than one that was never written.
		return fmt.Errorf("%w: output %q has an empty layout", ErrInvalid, o.Name)
	}
	for i, b := range o.Layout {
		if err := b.validate(o.Name, i, reportDefault); err != nil {
			return err
		}
	}
	return nil
}

func (b Block) validate(output string, i int, reportDefault string) error {
	switch {
	case !b.Kind.Valid():
		return fmt.Errorf("%w: %s block %d has kind %q, want stat, chart, table or text",
			ErrInvalid, output, i, b.Kind)
	case b.Kind != TextBlock && b.DatasetFor(reportDefault) == "":
		// Nothing to read. Rendering it empty would look like no data.
		return fmt.Errorf("%w: %s block %d names no dataset and the report has no default",
			ErrInvalid, output, i)
	case b.PageSize < 0:
		return fmt.Errorf("%w: %s block %d has a negative pageSize", ErrInvalid, output, i)
	}
	return b.validateShape(output, i)
}

// validateShape checks the fields each kind actually reads. A stat with no
// field renders a blank tile rather than failing, which is the worst outcome:
// it looks like an answer.
func (b Block) validateShape(output string, i int) error {
	switch b.Kind {
	case StatBlock:
		if b.Value.Field == "" {
			return fmt.Errorf("%w: %s stat %d measures no field", ErrInvalid, output, i)
		}
	case ChartBlock:
		return b.validateChart(output, i)
	case TableBlock:
		if len(b.Columns) == 0 {
			return fmt.Errorf("%w: %s table %d lists no columns", ErrInvalid, output, i)
		}
	case TextBlock:
		if b.Text == "" {
			return fmt.Errorf("%w: %s text %d has no text", ErrInvalid, output, i)
		}
	}
	return nil
}

// validateChart checks the fields the chart's own type reads.
//
// The type used to be an unchecked string, so `chart: pie` stored cleanly and
// failed at the viewer months later as "pie charts need a newer viewer" — a
// message that blames the reader's browser for the author's typo.
func (b Block) validateChart(output string, i int) error {
	switch {
	case b.Chart == "":
		return fmt.Errorf("%w: %s chart %d does not say what kind of chart — want one of %v",
			ErrInvalid, output, i, ChartTypeNames())
	case !b.Chart.Valid():
		return fmt.Errorf("%w: %s chart %d is a %q chart, want one of %v",
			ErrInvalid, output, i, b.Chart, ChartTypeNames())
	case b.Series.Field != "" && !b.Chart.MultiSeries():
		return fmt.Errorf("%w: %s chart %d splits by series, which a %s chart cannot draw",
			ErrInvalid, output, i, b.Chart)
	case b.Stacked && !b.Chart.Stacks():
		return fmt.Errorf("%w: %s chart %d is stacked, which a %s chart cannot be",
			ErrInvalid, output, i, b.Chart)
	case b.Target.Set() && !b.Chart.Folded():
		return fmt.Errorf("%w: %s chart %d sets a target, which only a gauge reads",
			ErrInvalid, output, i)
	case b.Stacked && b.Series.Field == "":
		// One series stacked against nothing is the same drawing, so this is
		// always a mistake rather than a no-op worth honouring silently.
		return fmt.Errorf("%w: %s chart %d is stacked but splits by no series",
			ErrInvalid, output, i)
	}

	switch {
	case b.Chart.Geographic():
		return b.validateMap(output, i)
	case b.Chart.Plots():
		return b.validatePlot(output, i)
	case b.Chart.Folded():
		return b.validateGauge(output, i)
	case len(b.Metrics) > 0:
		return b.validateMetrics(output, i)
	}
	if b.Chart.Metered() && b.Chart == ComboChart {
		return fmt.Errorf("%w: %s chart %d is a combo, which draws a list of metrics — "+
			"set metrics rather than y", ErrInvalid, output, i)
	}
	if b.X.Field == "" || b.Y.Field == "" {
		return fmt.Errorf("%w: %s chart %d needs both x and y", ErrInvalid, output, i)
	}
	if b.Chart.Gridded() && b.Series.Field == "" {
		// The second dimension is the other axis of the grid, not an optional
		// split: a heatmap with one dimension is a bar chart wearing squares.
		return fmt.Errorf("%w: %s chart %d is a heatmap, which needs series for its "+
			"second axis", ErrInvalid, output, i)
	}
	return nil
}

// validateMetrics checks a chart that reads a list of measures.
func (b Block) validateMetrics(output string, i int) error {
	if !b.Chart.Metered() {
		return fmt.Errorf("%w: %s chart %d lists metrics, which a %s chart does not read — "+
			"use y", ErrInvalid, output, i, b.Chart)
	}
	if b.Y.Field != "" {
		// Both would mean two answers to "what does this chart measure", and
		// honouring one silently makes the other look honoured.
		return fmt.Errorf("%w: %s chart %d sets both y and metrics", ErrInvalid, output, i)
	}
	mixes := b.Chart.Mixes()
	if mixes && len(b.Metrics) < 2 {
		return fmt.Errorf("%w: %s chart %d is a combo, which is two or more measures drawn "+
			"together — with one it is whichever chart that measure asked for",
			ErrInvalid, output, i)
	}
	for _, m := range b.Metrics {
		if err := m.validate(output, i, mixes); err != nil {
			return err
		}
	}
	// A combo puts its measures against buckets; a funnel's metrics *are* the
	// stages, so it has no x at all.
	if mixes && b.X.Field == "" {
		return fmt.Errorf("%w: %s chart %d needs x to say what its measures are drawn against",
			ErrInvalid, output, i)
	}
	if !mixes && b.X.Field != "" {
		return fmt.Errorf("%w: %s chart %d lists its stages as metrics, so x has nothing "+
			"to bucket", ErrInvalid, output, i)
	}
	return nil
}

// validateGauge checks a gauge, which folds the whole set to one number and
// reads it against a target.
func (b Block) validateGauge(output string, i int) error {
	switch {
	case b.Y.Field == "":
		return fmt.Errorf("%w: %s gauge %d measures no field", ErrInvalid, output, i)
	case b.X.Field != "":
		return fmt.Errorf("%w: %s gauge %d groups by %q — a gauge is one number, and "+
			"bucketing it would draw several", ErrInvalid, output, i, b.X.Field)
	}
	return b.Target.validate(output, i)
}

// validatePlot checks a scatter or bubble, whose x is a number and not a bucket.
func (b Block) validatePlot(output string, i int) error {
	switch {
	case b.X.Field == "":
		// One dot per category, not one per row. A scatter of raw observations
		// is a scatter of however many rows the dataset has, which is the one
		// number nobody bounded — and a browser asked to draw four million
		// circles is a tab that stops responding.
		return fmt.Errorf("%w: %s chart %d needs x to name what each dot is",
			ErrInvalid, output, i)
	case b.XValue.Field == "":
		return fmt.Errorf("%w: %s chart %d is a %s, so its horizontal axis is a measure — "+
			"set xValue rather than x", ErrInvalid, output, i, b.Chart)
	case b.Y.Field == "":
		return fmt.Errorf("%w: %s chart %d needs a y measure", ErrInvalid, output, i)
	case b.X.Grain != "":
		return fmt.Errorf("%w: %s chart %d sets a grain, which buckets a date — "+
			"a %s reads x as a number", ErrInvalid, output, i, b.Chart)
	case b.Chart == BubbleChart && b.Size.Field == "":
		return fmt.Errorf("%w: %s chart %d is a bubble, which sizes each dot by a "+
			"measure — set size, or use a scatter", ErrInvalid, output, i)
	case b.Chart == ScatterChart && b.Size.Field != "":
		return fmt.Errorf("%w: %s chart %d sets a size, which a scatter draws every "+
			"dot the same regardless — use a bubble", ErrInvalid, output, i)
	}
	return nil
}

// validateMap checks a map, whose axes are coordinates.
func (b Block) validateMap(output string, i int) error {
	if b.Map == nil {
		return fmt.Errorf("%w: %s chart %d is a map but has no map: block saying "+
			"what geography it reads", ErrInvalid, output, i)
	}
	if b.Y.Field == "" {
		// Every layer colours or sizes by something. A map with no measure is
		// an atlas, and this is a reporting tool.
		return fmt.Errorf("%w: %s map %d needs a y measure to colour by", ErrInvalid, output, i)
	}
	if b.Map.Draws(PolygonLayer) && b.Labels() == "" {
		// An unlabelled choropleth is a set of coloured shapes with no way to
		// say which is which, in a legend or a tooltip or an export.
		return fmt.Errorf("%w: %s map %d shades polygons but names nothing to label "+
			"them — set x, or map.region", ErrInvalid, output, i)
	}
	if err := b.validateFold(output, i); err != nil {
		return err
	}
	return b.Map.Validate(output, i)
}

// validateFold refuses the one aggregate that cannot survive being applied
// twice — see Block.Grouped.
//
// Only the aggregate written on the block is visible here; one inherited from
// the field's own default is resolved where the datasets are, so query re-runs
// this check with both in hand. Catching it at authoring time is still worth
// the duplication: the author who typed `avg` is the one who can pick.
func (b Block) validateFold(output string, i int) error {
	if b.Grouped() && Foldable(b.Y.Aggregate) == Unfoldable {
		return fmt.Errorf("%w: %s map %d shades polygons and draws points, so it runs "+
			"per point and adds each region up from those — which an average cannot "+
			"survive. Use sum, count, min or max, or split the layers across two blocks",
			ErrInvalid, output, i)
	}
	return nil
}

// Fold is how a polygon's value is recomputed from the point rows under it.
type Fold string

const (
	// Unfoldable means the aggregate gives a different answer applied twice.
	Unfoldable Fold = ""
	FoldSum    Fold = "sum"
	FoldMin    Fold = "min"
	FoldMax    Fold = "max"
)

// Foldable maps an aggregate to the function that re-folds its partial
// results. A sum of sums is a sum and a count of counts is a sum; a min of
// mins is a min; an average of averages is a number nobody measured.
func Foldable(aggregate string) Fold {
	switch aggregate {
	case "sum", "count", "":
		// Empty is the field's default, which is resolved elsewhere. Treating
		// it as foldable here keeps this check to the case an author can fix.
		return FoldSum
	case "min":
		return FoldMin
	case "max":
		return FoldMax
	}
	return Unfoldable
}
