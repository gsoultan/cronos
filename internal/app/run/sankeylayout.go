package run

import (
	"cmp"
	"slices"
)

const (
	// sankeyMost is as many nodes as a side draws before the smallest fold
	// into one: past it a node is a hairline with a name nobody can read.
	sankeyMost = 12
	// sankeyGap is the space between two nodes down a side, as a share of
	// the box.
	sankeyGap = 0.025
)

// layoutSankey lays a sankey out from its pairs — groups are the targets and
// their bars the sources, as a grouped chart reads them. Sources are ranked by
// what they send and targets by what they receive, largest first; each side
// is stacked down with a gap between its nodes; and each band takes its share
// of both its ends, sources in order into each target so no two bands cross
// inside a node. A pair of nothing or less flows nowhere and is not drawn.
func layoutSankey(groups []Group) *Sankey {
	out := &Sankey{Nodes: []SankeyNode{}, Links: []SankeyLink{}}
	flows, sources, targets := foldFlows(groups)
	grand := 0.0
	for _, s := range sources {
		grand += s.value
	}
	if grand <= 0 {
		return out
	}
	gaps := float64(max(len(sources), len(targets))-1) * sankeyGap
	k := (1 - gaps) / grand
	placeSide(out, sources, 0, k)
	placeSide(out, targets, 1, k)
	used := make([]float64, len(out.Nodes))
	for s := range sources {
		for t := range targets {
			v := flows[s][t]
			if v <= 0 {
				continue
			}
			from, to := s, len(sources)+t
			h := v * k
			out.Links = append(out.Links, SankeyLink{From: from, To: to,
				Y0: out.Nodes[from].Y + used[from], Y1: out.Nodes[to].Y + used[to],
				H: h, Value: v, Formatted: compact(v)})
			used[from] += h
			used[to] += h
		}
	}
	return out
}

// side is one node before it is placed: what it is called and carries.
type side struct {
	label string
	value float64
}

// placeSide stacks one side's nodes down the box, centred, at k of the box
// per unit carried.
func placeSide(out *Sankey, nodes []side, at int, k float64) {
	total := float64(len(nodes)-1) * sankeyGap
	for _, n := range nodes {
		total += n.value * k
	}
	y := (1 - total) / 2
	for i, n := range nodes {
		out.Nodes = append(out.Nodes, SankeyNode{Label: n.label, Side: at, Y: y, H: n.value * k,
			Value: n.value, Formatted: compact(n.value), Slot: min(i, CategorySlots-1)})
		y += n.value*k + sankeyGap
	}
}

// foldFlows is the pairs as a source-by-target table, each side ranked
// largest first and folded to sankeyMost, the smallest into "Other".
func foldFlows(groups []Group) ([][]float64, []side, []side) {
	var sources, targets []side
	if len(groups) > 0 {
		for _, b := range groups[0].Bars {
			sources = append(sources, side{label: b.Label})
		}
	}
	for t, g := range groups {
		targets = append(targets, side{label: g.Label})
		for s, b := range g.Bars {
			if b.Value > 0 {
				sources[s].value += b.Value
				targets[t].value += b.Value
			}
		}
	}
	srcAt, sources := rankFold(sources)
	tgtAt, targets := rankFold(targets)
	flows := make([][]float64, len(sources))
	for i := range flows {
		flows[i] = make([]float64, len(targets))
	}
	for t, g := range groups {
		for s, b := range g.Bars {
			if b.Value > 0 {
				flows[srcAt[s]][tgtAt[t]] += b.Value
			}
		}
	}
	return flows, sources, targets
}

// rankFold orders a side largest first, drops what carries nothing, and folds
// the smallest past sankeyMost into one — returning where each of the
// original nodes went, -1 for none, and the side as drawn.
func rankFold(nodes []side) ([]int, []side) {
	order := make([]int, len(nodes))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(nodes[b].value, nodes[a].value) })
	carrying := 0
	for _, n := range nodes {
		if n.value > 0 {
			carrying++
		}
	}
	at := make([]int, len(nodes))
	var out []side
	for _, i := range order {
		switch {
		case nodes[i].value <= 0:
			at[i] = -1
		case carrying <= sankeyMost || len(out) < sankeyMost-1:
			at[i] = len(out)
			out = append(out, nodes[i])
		default:
			if len(out) == sankeyMost-1 {
				out = append(out, side{label: "Other"})
			}
			at[i] = sankeyMost - 1
			out[sankeyMost-1].value += nodes[i].value
		}
	}
	return at, out
}
