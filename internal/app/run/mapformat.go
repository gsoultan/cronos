package run

import "github.com/gsoultan/cronos/internal/core/definition"

/*
writeAs gives a map's figures the format its measure declares: each region's,
place's, flow's, route's and hexagon's value, the same in every period of a
timed map, and the legend's bounds — so a map of revenue says "$2.4M" in its
tooltip and its key, as the bar chart beside it does, rather than "2.4M".

Each layer of a map reads the block's one measure, which is why one format
does for all of them; an overlay is a block of its own and is written in its
own dataset's format when it is read. A bubble's size is a second measure and
stays as it was read. Cells — the gathered places of a map too large to send
one by one — carry numbers rather than words, and a viewer words them.
*/
func (m *GeoMap) writeAs(blk definition.Block, ds definition.Dataset) {
	if m == nil {
		return
	}
	nf := formatOf(ds, blk.Y.Field)
	if nf.plain() {
		return
	}
	for _, list := range [][]Shape{m.Shapes, m.Lines, m.Hexes} {
		for i := range list {
			list[i].Formatted = nf.short(list[i].Value)
			writeFrames(list[i].Frames, nf)
		}
	}
	for i := range m.Markers {
		m.Markers[i].Formatted = nf.short(m.Markers[i].Value)
		writeFrames(m.Markers[i].Frames, nf)
	}
	for i := range m.Arcs {
		m.Arcs[i].Formatted = nf.short(m.Arcs[i].Value)
		writeFrames(m.Arcs[i].Frames, nf)
	}
	writeLegend(m.Legend, nf)
	writeLegend(m.FrameLegend, nf)
}

func writeFrames(f Frames, nf numberFormat) {
	for k, v := range f {
		v.Formatted = nf.short(v.Value)
		f[k] = v
	}
}

func writeLegend(l []Legend, nf numberFormat) {
	for i := range l {
		l[i].From, l[i].To = nf.short(l[i].lower), nf.short(l[i].upper)
	}
}
