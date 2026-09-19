package run

import (
	"math"
	"sort"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// item is one rectangle waiting to be placed.
type item struct {
	label string
	group string
	value float64
}

// readTreemap reads (bucket[, group], value) into laid-out rectangles.
//
// One level when the block names no series, two when it does — the groups
// become outer rectangles and each is squarified again inside its own box.
// That nesting is the whole reason to reach for a treemap over a pie: it holds
// "revenue by customer, within region" without needing a second chart.
func readTreemap(blk definition.Block, rows Rows) ([]Rect, error) {
	nested := blk.Series.Field != ""
	width := 2
	if nested {
		width = 3
	}

	var items []item
	for rows.Next() {
		cells := make([]any, width)
		into := make([]any, width)
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := rows.Scan(into...); err != nil {
			return nil, err
		}
		it := item{label: bucketLabel(cells[0], blk.X.Grain)}
		if nested {
			it.group = cell(cells[1])
			it.value, _ = number(cells[2])
		} else {
			it.value, _ = number(cells[1])
		}
		// A treemap encodes quantity as area, and an area cannot be negative.
		// Dropping the row is honest; drawing its magnitude would say the
		// opposite of what the number means.
		if it.value > 0 {
			items = append(items, it)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if nested {
		return nest(items), nil
	}
	return squarify(items, box{0, 0, 1, 1}, "", 0), nil
}

// nest lays the groups out, then each group's leaves inside its box.
func nest(items []item) []Rect {
	var order []string
	totals := map[string]float64{}
	inside := map[string][]item{}
	for _, it := range items {
		if _, seen := totals[it.group]; !seen {
			order = append(order, it.group)
		}
		totals[it.group] += it.value
		inside[it.group] = append(inside[it.group], it)
	}

	groups := make([]item, 0, len(order))
	for _, g := range order {
		groups = append(groups, item{label: g, value: totals[g]})
	}

	out := []Rect{}
	// The group frames take categorical slots — swapping two regions does not
	// change what the chart says — and every leaf takes its group's slot, so a
	// reader sees at a glance which rectangles belong together.
	for slot, frame := range squarify(groups, box{0, 0, 1, 1}, "", 0) {
		frame.Slot = slot
		out = append(out, frame)
		out = append(out, squarify(inside[frame.Label],
			box{frame.X, frame.Y, frame.W, frame.H}, frame.Label, slot)...)
	}
	return out
}

// box is a rectangle being filled.
type box struct{ x, y, w, h float64 }

// squarify lays items out to keep each rectangle as close to square as it can.
//
// The published algorithm (Bruls, Huizing & van Wijk): take items largest
// first, keep adding to the current row while doing so improves the worst
// aspect ratio in it, and lay the row down when it stops. Rectangles near a
// square are the only ones a reader can compare by area — a 40:1 sliver
// carries its value in a dimension the eye does not read.
//
// Coordinates come out as fractions of the unit square, so a viewer scales
// them to whatever width it has without re-laying anything out.
func squarify(items []item, into box, group string, slot int) []Rect {
	total := 0.0
	for _, it := range items {
		total += it.value
	}
	if total <= 0 || into.w <= 0 || into.h <= 0 {
		return []Rect{}
	}

	sorted := append([]item(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].value > sorted[j].value })

	// Scaled to the box's area, so a row's length is directly its share.
	scale := (into.w * into.h) / total
	out := make([]Rect, 0, len(sorted))
	free := into
	var row []item

	for _, it := range sorted {
		if len(row) > 0 && worst(append(row, it), free, scale) > worst(row, free, scale) {
			free = lay(row, free, scale, group, slot, &out)
			row = row[:0]
		}
		row = append(row, it)
	}
	if len(row) > 0 {
		lay(row, free, scale, group, slot, &out)
	}
	return out
}

// worst is the least square-like aspect ratio a row would have.
func worst(row []item, free box, scale float64) float64 {
	sum, lo, hi := 0.0, math.Inf(1), 0.0
	for _, it := range row {
		a := it.value * scale
		sum += a
		lo, hi = math.Min(lo, a), math.Max(hi, a)
	}
	if sum <= 0 || lo <= 0 {
		return math.Inf(1)
	}
	side := math.Min(free.w, free.h)
	return math.Max(side*side*hi/(sum*sum), sum*sum/(side*side*lo))
}

// lay places one row along the short side and returns what is left.
//
// The short side, always, because that is what keeps the rectangles square:
// filling along the long side of a wide box makes every item in the row tall
// and thin, which is the shape the algorithm exists to avoid.
func lay(row []item, free box, scale float64, group string, slot int, out *[]Rect) box {
	sum := 0.0
	for _, it := range row {
		sum += it.value * scale
	}
	// A vertical strip when the free box is wider than it is tall.
	vertical := free.w >= free.h
	along := free.h
	if !vertical {
		along = free.w
	}
	thickness := sum / math.Max(along, 1e-12)

	at := 0.0
	for i, it := range row {
		r := Rect{
			Label: it.label, Value: it.value, Formatted: compact(it.value),
			Group: group, Slot: slot, Depth: 1,
		}
		if group == "" {
			// A top-level rectangle: its slot is its rank, largest first,
			// which is the order the ordinal ramp encodes.
			r.Depth, r.Slot = 0, len(*out)+i
		}
		length := (it.value * scale) / math.Max(thickness, 1e-12)
		if vertical {
			r.X, r.Y, r.W, r.H = free.x, free.y+at, thickness, length
		} else {
			r.X, r.Y, r.W, r.H = free.x+at, free.y, length, thickness
		}
		at += length
		*out = append(*out, r)
	}

	if vertical {
		return box{free.x + thickness, free.y, free.w - thickness, free.h}
	}
	return box{free.x, free.y + thickness, free.w, free.h - thickness}
}
