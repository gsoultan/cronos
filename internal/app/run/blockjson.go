package run

import "encoding/json"

// MarshalJSON emits only the fields the block's kind owns, and always emits
// those.
//
// Neither of the obvious spellings works. Plain tags put `"series": null` on
// every stat tile — noise a client has to know to ignore. `omitempty` drops an
// *empty* collection too, so a chart that matched no rows loses the key
// entirely and a client reading `series.map` crashes on the emptiest, most
// ordinary case there is.
//
// A client should never have to distinguish absent from empty, so for the kind
// that owns a collection it is always there, and for the kinds that do not it
// never is.
func (b Block) MarshalJSON() ([]byte, error) {
	out := map[string]any{"kind": b.Kind, "title": b.Title}
	if b.Coverage != nil {
		out["coverage"] = b.Coverage
	}

	switch b.Kind {
	case "stat", "text":
		out["value"] = b.Value
	case "chart":
		out["chart"] = b.Chart
		out["series"] = nonNilBars(b.Series)
		b.chartJSON(out)
	case "table":
		out["columns"] = nonNilColumns(b.Columns)
		out["rows"] = nonNilRows(b.Rows)
		out["total"] = b.Total
	}
	return json.Marshal(out)
}

func nonNilBars(v []Bar) []Bar {
	if v == nil {
		return []Bar{}
	}
	return v
}

func nonNilColumns(v []Column) []Column {
	if v == nil {
		return []Column{}
	}
	return v
}

func nonNilRows(v [][]string) [][]string {
	if v == nil {
		return [][]string{}
	}
	return v
}

// chartJSON adds the keys the chart's own type owns.
//
// The same rule as the switch above, one level down: a chart type that carries
// a collection always emits it, and one that does not never does. `series` is
// emitted for every chart regardless, because it is the chart *kind's*
// collection and a viewer reading `series.map` predates knowing there was a
// type to ask about.
func (b Block) chartJSON(out map[string]any) {
	switch {
	case b.Map != nil:
		out["map"] = b.Map
	case b.Points != nil:
		out["points"] = b.Points
		out["xAxis"] = b.XAxis
	case b.Groups != nil:
		out["groups"] = b.Groups
		out["stacked"] = b.Stacked
		if b.Stacked {
			out["totals"] = b.Totals
		}
	case b.Gauge != nil:
		out["gauge"] = b.Gauge
	case b.Tracks != nil:
		out["tracks"] = b.Tracks
		if b.Axis2 != nil {
			out["axis2"] = b.Axis2
		}
	case b.Stages != nil:
		out["stages"] = b.Stages
	case b.Steps != nil:
		out["steps"] = b.Steps
	case b.Cells != nil:
		out["cells"] = b.Cells
		out["heatRows"] = b.HeatRows
		out["heatColumns"] = b.HeatColumns
	case b.Rects != nil:
		out["rects"] = b.Rects
	}

	// The vertical scale cuts across the shapes above rather than belonging to
	// one of them: a line carries groups or a series, a scatter carries
	// points, and both are read against a measured axis. A bar has none, which
	// is why this asks whether there is one rather than which shape it is.
	if b.YAxis != nil {
		out["yAxis"] = b.YAxis
	}
}
