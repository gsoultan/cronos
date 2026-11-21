package run

import "github.com/gsoultan/cronos/internal/core/document"

// facetPanelMM is how tall a small multiple's box is: a third of a chart's,
// which is what three across a page leaves the box once it keeps its width
// to height.
const facetPanelMM = 24

// facetTicks is about how many labels a panel's bottom scale keeps. A third
// of the page holds four months' names; twelve were set over each other.
const facetTicks = 4

// facetsOf prints a chart split into small multiples: each group drawn as a
// chart of its own, named by the group, against the shared scale the block
// carries — so a panel that looks taller is larger, not a different axis.
func facetsOf(b Block) document.Chart {
	out := document.Chart{Title: b.Title, Kind: b.Chart, Marks: []document.Mark{}}
	for _, g := range b.Groups {
		one := b
		one.Title, one.Facets = g.Label, false
		one.Groups = []Group{g}
		one.Totals = nil
		p := chartOf(one)
		p.Height = facetPanelMM
		// One series a panel: the panel's name says which, so a key under
		// each would say it a second time.
		p.Keys = nil
		p.XTicks = thinned(p.XTicks, facetTicks)
		out.Panels = append(out.Panels, p)
	}
	return out
}

// thinned keeps every so many ticks, from the first, so about most remain.
func thinned(ticks []document.Tick, most int) []document.Tick {
	if len(ticks) <= most {
		return ticks
	}
	every := (len(ticks) + most - 1) / most
	out := make([]document.Tick, 0, most)
	for i := 0; i < len(ticks); i += every {
		out = append(out, ticks[i])
	}
	return out
}
