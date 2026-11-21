package run

import "github.com/gsoultan/cronos/internal/core/definition"

/*
Formats are applied in one pass over a block once it is read, rather than at
each of the forty places a figure is first written. Those places write a figure
plainly; this rewrites the ones whose field asked for something else, from the
number each mark already carries. One pass is one place to look when a figure
reads wrong, and a reader added later is formatted without having to know that
formats exist. A field that asks for nothing is left exactly as it was read.

Maps are not rewritten: their figures are drawn from several datasets' fields
per layer, and stay plain for now.
*/

// writeAs rewrites the block's figures in the formats its fields declare.
func (b *Block) writeAs(blk definition.Block, ds definition.Dataset) {
	switch blk.Kind {
	case definition.StatBlock:
		nf := formatOf(ds, blk.Value.Field)
		if b.figure != nil && !nf.plain() {
			b.Value = nf.short(*b.figure)
		}
		writeBars(b.Trend, nf)
		if b.Goal != nil && !nf.plain() {
			b.Goal.Value = nf.short(b.Goal.target)
		}
	case definition.ChartBlock:
		b.writeChart(blk, ds)
	}
}

func (b *Block) writeChart(blk definition.Block, ds definition.Dataset) {
	y := formatOf(ds, blk.Y.Field)
	switch {
	case blk.Chart.Geographic():
		return
	case blk.Chart.Plots():
		x := formatOf(ds, blk.XValue.Field)
		for i := range b.Points {
			p := &b.Points[i]
			p.FX, p.FY = writeOne(x, p.X, p.FX), writeOne(y, p.Y, p.FY)
		}
		writeAxis(b.XAxis, x)
	case blk.Chart.Mixes():
		b.writeTracks(blk, ds)
		return
	case blk.Chart == definition.ParetoChart:
		// The columns are the measure; the line is a share, written as one
		// when it was read.
		if len(b.Tracks) > 0 {
			writeBars(b.Tracks[0].Bars, y)
		}
		writeRefs(b.References, y)
		writeAxis(b.YAxis, y)
		return
	case blk.Chart == definition.FunnelChart:
		b.writeStages(blk, ds, y)
	case blk.Chart == definition.HistogramChart:
		b.writeBins(formatOf(ds, blk.X.Field), y)
	}
	b.writeMarks(y)
	writeRefs(b.References, y)
	if !b.Percent {
		writeAxis(b.YAxis, y)
	}
	if blk.Chart == definition.BulletChart {
		writeAxis(b.XAxis, y)
	}
}

// writeMarks rewrites every mark that reads the block's one measure.
func (b *Block) writeMarks(nf numberFormat) {
	if nf.plain() {
		return
	}
	writeBars(b.Series, nf)
	writeBars(b.Totals, nf)
	for i := range b.Groups {
		writeBars(b.Groups[i].Bars, nf)
	}
	for i := range b.Steps {
		b.Steps[i].Formatted = nf.short(b.Steps[i].Value)
	}
	for i := range b.Cells {
		if !b.Cells[i].Empty {
			b.Cells[i].Formatted = nf.short(b.Cells[i].Value)
		}
	}
	for i := range b.Rects {
		b.Rects[i].Formatted = nf.short(b.Rects[i].Value)
	}
	for i := range b.Days {
		b.Days[i].Formatted = nf.short(b.Days[i].Value)
	}
	b.writeMeasured(nf)
}

// writeMeasured rewrites the marks that carry more than one figure each.
func (b *Block) writeMeasured(nf numberFormat) {
	if g := b.Gauge; g != nil {
		g.Formatted, g.TargetFormatted = nf.short(g.Value), nf.short(g.Target)
	}
	for i := range b.Bullets {
		r := &b.Bullets[i]
		r.Formatted, r.TargetFormatted = nf.short(r.Value), nf.short(r.Target)
	}
	for i := range b.Boxes {
		x := &b.Boxes[i]
		x.Said = [5]string{nf.short(x.Low), nf.short(x.Q1), nf.short(x.Median),
			nf.short(x.Q3), nf.short(x.High)}
	}
	if s := b.Sankey; s != nil {
		for i := range s.Nodes {
			s.Nodes[i].Formatted = nf.short(s.Nodes[i].Value)
		}
		for i := range s.Links {
			s.Links[i].Formatted = nf.short(s.Links[i].Value)
		}
	}
}

// writeTracks gives each measure of a combo its own format, and each scale
// the format of the first measure read against it.
func (b *Block) writeTracks(blk definition.Block, ds definition.Dataset) {
	var left, right *numberFormat
	for i := range b.Tracks {
		if i >= len(blk.Metrics) {
			break
		}
		nf := formatOf(ds, blk.Metrics[i].Field)
		writeBars(b.Tracks[i].Bars, nf)
		switch {
		case b.Tracks[i].Secondary && right == nil:
			right = &nf
		case !b.Tracks[i].Secondary && left == nil:
			left = &nf
		}
	}
	if left != nil {
		writeAxis(b.YAxis, *left)
		writeRefs(b.References, *left)
	}
	if right != nil {
		writeAxis(b.Axis2, *right)
	}
}

// writeStages formats a funnel's stages: by each stage's own measure where
// the stages are measures, and by the one measure where they are rows.
func (b *Block) writeStages(blk definition.Block, ds definition.Dataset, y numberFormat) {
	for i := range b.Stages {
		nf := y
		if len(blk.Metrics) > 0 && i < len(blk.Metrics) {
			nf = formatOf(ds, blk.Metrics[i].Field)
		}
		b.Stages[i].Formatted = writeOne(nf, b.Stages[i].Value, b.Stages[i].Formatted)
	}
}

// writeBins formats a histogram: its edges are the binned number's, and its
// heights the measure folded over each bin's rows, or a count.
func (b *Block) writeBins(x, y numberFormat) {
	for i := range b.Bins {
		bin := &b.Bins[i]
		if !x.plain() {
			bin.Label = x.short(bin.From) + "–" + x.short(bin.To)
		}
		bin.Formatted = writeOne(y, bin.Value, bin.Formatted)
	}
	writeAxis(b.XAxis, x)
}

func writeBars(bars []Bar, nf numberFormat) {
	if nf.plain() {
		return
	}
	for i := range bars {
		bars[i].Formatted = nf.short(bars[i].Value)
	}
}

// writeOne is v in nf, or what it already said where nf is plain.
func writeOne(nf numberFormat, v float64, said string) string {
	if nf.plain() {
		return said
	}
	return nf.short(v)
}

// writeAxis relabels a scale's ticks from where each one sits on it.
func writeAxis(a *Axis, nf numberFormat) {
	if a == nil || nf.plain() {
		return
	}
	for i := range a.Ticks {
		t := &a.Ticks[i]
		t.Label = nf.tick(a.Min + t.At*(a.Max-a.Min))
	}
}

func writeRefs(refs []RefLine, nf numberFormat) {
	if nf.plain() {
		return
	}
	for i := range refs {
		refs[i].Formatted = nf.short(refs[i].Value)
	}
}
