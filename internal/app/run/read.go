package run

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// read turns a result set into the block a viewer draws.
//
// Split by kind rather than one generic reader, because the three shapes want
// genuinely different things: a stat wants one cell, a chart wants two
// columns, a table wants all of them as text.
func read(blk definition.Block, ds definition.Dataset, rows Rows, cov *Coverage) (Block, error) {
	out := Block{Kind: string(blk.Kind), Title: blk.Heading(), Coverage: cov}

	var err error
	switch blk.Kind {
	case definition.StatBlock:
		out.Value, err = readStat(rows)
	case definition.ChartBlock:
		out.Chart = string(blk.Chart)
		err = readChart(&out, blk, ds, rows)
	case definition.TableBlock:
		out.Columns, out.Rows, err = readTable(blk, ds, rows)
		out.Total = len(out.Rows)
	default:
		err = fmt.Errorf("%w: cannot read a %q block", ErrNotRenderable, blk.Kind)
	}
	if err != nil {
		return Block{}, err
	}
	return out, rows.Err()
}

// readChart reads whichever shape the chart's type asked the compiler for.
//
// Series stays an empty slice on the types that do not use it, rather than
// nil: it is the chart kind's collection, and a viewer reading `series.map`
// should not have to know which types filled it in.
func readChart(out *Block, blk definition.Block, ds definition.Dataset, rows Rows) error {
	out.Series = []Bar{}
	var err error
	switch {
	case blk.Chart.Geographic():
		out.Map, err = readMap(blk, ds, rows)
	case blk.Chart.Plots():
		var x, y Axis
		out.Points, x, y, err = readPoints(rows, blk.Series.Field != "", blk.Size.Field != "")
		out.XAxis, out.YAxis = &x, &y
	case blk.Chart.Folded():
		out.Gauge, err = readGauge(blk, rows)
	case blk.Chart.Mixes():
		var y Axis
		out.Tracks, y, out.Axis2, err = readCombo(blk, rows)
		out.YAxis = &y
	case blk.Chart == definition.FunnelChart:
		out.Stages, err = readFunnel(blk, rows)
	case blk.Chart == definition.WaterfallChart:
		var y Axis
		out.Steps, y, err = readWaterfall(blk, rows)
		out.YAxis = &y
	case blk.Chart.Gridded():
		var g grid
		if g, err = readHeatmap(blk, rows); err == nil {
			out.Cells, out.HeatRows, out.HeatColumns = g.cells, g.rows, g.columns
			// Total is the grid the data asked for, which may exceed the one
			// drawn — the same promise a table's Total makes about its page.
			if g.full > len(g.cells) {
				out.Total = g.full
			}
		}
	case blk.Chart == definition.TreemapChart:
		out.Rects, err = readTreemap(blk, rows)
	case blk.Series.Field != "":
		out.Stacked = blk.Stacked
		out.Groups, err = readGroups(rows, blk.X.Grain)
		if blk.Stacked {
			out.Totals = totals(out.Groups)
		}
	default:
		out.Series, err = readSeries(rows, blk.X.Grain)
	}
	if err == nil {
		out.scaleFor(blk.Chart)
	}
	return err
}

// scaleFor gives the chart types that draw against a measured axis one, with
// its ticks already formatted.
//
// A bar needs none: the number sits beside each bar, and a bar drawn against
// the largest value ranks while its label states. A line has no room for a
// label per point, so the axis is the only thing carrying the magnitude — and
// a viewer choosing its own ticks would be the one part of the report this
// engine did not format.
func (b *Block) scaleFor(chart definition.ChartType) {
	if chart != definition.LineChart && chart != definition.AreaChart {
		return
	}
	var values []float64
	switch {
	case b.Totals != nil:
		// A stacked area is read at its top edge, so the scale has to reach
		// the stack's height rather than its tallest single series.
		for _, t := range b.Totals {
			values = append(values, t.Value)
		}
	case b.Groups != nil:
		for _, g := range b.Groups {
			for _, bar := range g.Bars {
				values = append(values, bar.Value)
			}
		}
	default:
		for _, bar := range b.Series {
			values = append(values, bar.Value)
		}
	}
	y := axis(values)
	b.YAxis = &y
}

// readStat takes the single cell the aggregate produced.
func readStat(rows Rows) (string, error) {
	if !rows.Next() {
		// No row at all, which an aggregate should not do — but a stat that
		// renders nothing looks like a number that happens to be blank.
		return "—", nil
	}
	var v any
	if err := rows.Scan(&v); err != nil {
		return "", err
	}
	n, ok := number(v)
	if !ok {
		// Nothing matched. Zero would be a claim; an em dash is the absence.
		return "—", nil
	}
	return compact(n), nil
}

// readSeries reads (bucket, value) pairs.
func readSeries(rows Rows, grain string) ([]Bar, error) {
	out := []Bar{}
	for rows.Next() {
		var label, value any
		if err := rows.Scan(&label, &value); err != nil {
			return nil, err
		}
		n, _ := number(value)
		out = append(out, Bar{Label: bucketLabel(label, grain), Value: n, Formatted: compact(n)})
	}
	return out, nil
}

// readTable reads every column as text.
func readTable(blk definition.Block, ds definition.Dataset, rows Rows) ([]Column, [][]string, error) {
	names, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	cols := headings(blk, ds, names)

	out := [][]string{}
	for rows.Next() {
		cells := make([]any, len(names))
		into := make([]any, len(names))
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := rows.Scan(into...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(cells))
		for i, c := range cells {
			row[i] = cell(c)
		}
		out = append(out, row)
	}
	return cols, out, nil
}

// headings labels the columns from the dataset, so a table says "Customer"
// rather than "customer_name". The field name is the contract reports bind to;
// the label is what people read.
func headings(blk definition.Block, ds definition.Dataset, names []string) headers {
	cols := make(headers, 0, len(names))
	for i, name := range names {
		if i < len(blk.Columns) {
			name = blk.Columns[i]
		}
		col := Column{Label: name}
		if f, ok := ds.Field(name); ok {
			if f.Label != "" {
				col.Label = f.Label
			}
			if f.Role == definition.Measure {
				col.Align = "right"
			}
		}
		cols = append(cols, col)
	}
	return cols
}

// headers is a list of headings, so a caller wanting only the labels does not
// have to loop at the call site.
type headers []Column

func (c headers) labels() []string {
	out := make([]string, len(c))
	for i, col := range c {
		out[i] = col.Label
	}
	return out
}
