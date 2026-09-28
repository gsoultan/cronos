package run

import (
	"sort"

	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
timeline gathers a timed map's marks period by period, as the rows arrive a
place and a period at a time.

A region is known by its label, as it already is; a place by its label, its
category and where it is, so its rows in each period fold into one marker
rather than one per period; a route by its label and both ends. Every mark
also keeps its total over all periods, which is the map as it opens.
*/
type timeline struct {
	grain   string
	index   map[string]int
	raw     []any
	regions map[string]map[int]float64
	places  map[placeKey]int
	routes  map[routeKey]int
	marker  []map[int]float64
	arc     []map[int]float64
	// size is each place's size measure by period, and sizes its total over
	// every period, folded as the measure's own aggregate folds; both nil on
	// a map without one.
	size     []map[int]float64
	sizes    []float64
	sizeFold definition.Fold
}

type placeKey struct {
	label, series string
	x, y          float64
}

type routeKey struct {
	label, series  string
	x1, y1, x2, y2 float64
}

func newTimeline(t *definition.DimensionRef, sizeFold definition.Fold) *timeline {
	return &timeline{grain: t.Grain, index: map[string]int{}, sizeFold: sizeFold,
		regions: map[string]map[int]float64{}, places: map[placeKey]int{}, routes: map[routeKey]int{}}
}

// place starts the periods of the place at marker i.
func (t *timeline) place(key placeKey, i, now int, value, size float64, sized bool) {
	t.places[key] = i
	t.marker = append(t.marker, map[int]float64{now: value})
	if sized {
		t.size = append(t.size, map[int]float64{now: size})
		t.sizes = append(t.sizes, size)
	}
}

// sized folds a place's size in a period into that period and into its
// total, and returns the total.
func (t *timeline) sized(i, now int, size float64) float64 {
	addPeriod(t.size[i], now, size, t.sizeFold)
	t.sizes[i] = refold(t.sizeFold, t.sizes[i], size)
	return t.sizes[i]
}

// period is the index of a row's period, in order of first appearance — or
// -1 for a row with no date, which counts in the map as it opens and in no
// period: a period named nothing is not one a reader can play.
func (t *timeline) period(raw any) int {
	if raw == nil {
		return -1
	}
	key := cell(raw)
	if i, ok := t.index[key]; ok {
		return i
	}
	t.index[key] = len(t.raw)
	t.raw = append(t.raw, raw)
	return len(t.raw) - 1
}

func (t *timeline) region(label string, period int, value float64, fold definition.Fold) {
	byPeriod, ok := t.regions[label]
	if !ok {
		byPeriod = map[int]float64{}
		t.regions[label] = byPeriod
	}
	addPeriod(byPeriod, period, value, fold)
}

// addPeriod folds a value into a mark's total for one period.
func addPeriod(byPeriod map[int]float64, period int, value float64, fold definition.Fold) {
	if v, seen := byPeriod[period]; seen {
		value = refold(fold, v, value)
	}
	byPeriod[period] = value
}

// order is the periods in time, as indexes in the order they were first seen.
//
// Every one of them. A map that played the latest few would open on totals
// over periods its reader could not play, and nothing here needs the cap:
// frames are kept only where a mark had rows, so there are no more of them
// than the rows the map read.
func (t *timeline) order() []int {
	out := make([]int, len(t.raw))
	for i := range out {
		out[i] = i
	}
	sort.SliceStable(out, func(a, b int) bool {
		ta, okA := asTime(t.raw[out[a]])
		tb, okB := asTime(t.raw[out[b]])
		if okA && okB {
			return ta.Before(tb)
		}
		return cell(t.raw[out[a]]) < cell(t.raw[out[b]])
	})
	return out
}

// finish writes each mark's periods, and the periods' names, onto the map.
func (t *timeline) finish(out *GeoMap, m *definition.MapSpec) {
	order := t.order()
	out.Frames = make([]string, len(order))
	for i, p := range order {
		out.Frames[i] = bucketLabel(t.raw[p], t.grain)
	}
	t.shadeFrames(out, m, order)
	t.sizeFrames(out, order)
}

// shadeFrames gives each region and route its value and shade in every
// period, over one set of shades for all of them: a colour means one value in
// every period, so a period is compared with the last by its colours.
func (t *timeline) shadeFrames(out *GeoMap, m *definition.MapSpec, order []int) {
	var values []float64
	for _, byPeriod := range t.regions {
		for _, p := range order {
			if v, ok := byPeriod[p]; ok {
				values = append(values, v)
			}
		}
	}
	ramp := shadesFor(m, values)
	mark := func(_ int, v float64) FrameMark {
		return FrameMark{Value: v, Formatted: compact(v), Step: ramp.shade(v)}
	}
	for _, list := range [][]Shape{out.Shapes, out.Lines} {
		for i := range list {
			list[i].Frames = framesOf(t.regions[list[i].Label], order, mark)
		}
	}
	if len(values) > 0 && (len(out.Shapes) > 0 || len(out.Lines) > 0) {
		out.FrameLegend = ramp.legend(values)
	}
}

// sizeFrames gives each place and route its value and size in every period,
// sized against the largest in any of them.
func (t *timeline) sizeFrames(out *GeoMap, order []int) {
	var values []float64
	for _, list := range [][]map[int]float64{t.marker, t.arc} {
		for _, byPeriod := range list {
			for _, p := range order {
				if v, ok := byPeriod[p]; ok {
					values = append(values, v)
				}
			}
		}
	}
	lo, hi := span(values)
	mark := func(_ int, v float64) FrameMark {
		return FrameMark{Value: v, Formatted: compact(v), Weight: weight(v, lo, hi)}
	}
	for i := range out.Markers {
		out.Markers[i].Frames = framesOf(t.marker[i], order, func(p int, v float64) FrameMark {
			f := mark(p, v)
			if t.size != nil {
				f.Size = compact(t.size[i][p])
			}
			return f
		})
	}
	for i := range out.Arcs {
		out.Arcs[i].Frames = framesOf(t.arc[i], order, mark)
	}
}

// framesOf is a mark's value in each period it had rows in, keyed by the
// period's place in order.
func framesOf(byPeriod map[int]float64, order []int, mark func(period int, v float64) FrameMark) Frames {
	out := make(Frames, len(byPeriod))
	for i, p := range order {
		if v, ok := byPeriod[p]; ok {
			out[i] = mark(p, v)
		}
	}
	return out
}
