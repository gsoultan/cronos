package run

import (
	"math"
	"sort"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/platform/h3"
)

// h3Unsized is the resolution places are binned at when there is no span to
// size cells to — one place, or several at one spot: cells a few hundred
// metres across.
const h3Unsized = 9

/*
h3Cells gathers an h3 layer's cells as a map's rows arrive.

Cells a warehouse has indexed its rows by are folded in as they come, rolled
up to a coarser resolution where the map asks for one — by parent, as H3 rolls
cells up, since a cell's middle is not always in its parent. Places are kept
until every one has been seen, because the resolution they are binned at is
sized to all of them.
*/
type h3Cells struct {
	spec   *definition.MapSpec
	fold   definition.Fold
	sums   map[uint64]*hexSum
	binned bool
	// res is what places are binned at: the author's, the one sizeTo found,
	// or -1 while it waits for every place — which are kept meanwhile.
	res    int
	places []h3Place
}

// h3Place is one place to bin: where, and its value.
type h3Place struct{ lat, lon, value float64 }

func newH3Cells(m *definition.MapSpec, fold definition.Fold) *h3Cells {
	res := m.H3Resolution
	if res == 0 {
		res = -1
	}
	return &h3Cells{spec: m, fold: fold, sums: map[uint64]*hexSum{}, binned: m.H3 == "", res: res}
}

// sizeTo fixes the resolution places are binned at from the box they span,
// where the author set none, and bins any kept waiting for it.
func (c *h3Cells) sizeTo(points *Bounds) {
	if c.res < 0 {
		c.res = h3Fit(points)
	}
	for _, p := range c.places {
		c.place(p.lat, p.lon, p.value)
	}
	c.places = nil
}

// cell folds one row's indexed cell in. A value that is not a cell is left
// out, as a place with no coordinates is: a column holds somebody's data.
func (c *h3Cells) cell(raw any, value float64) {
	h, ok := cellID(raw)
	if !ok {
		return
	}
	if res := c.spec.H3Resolution; res > 0 {
		h = h3.Parent(h, res)
	}
	c.add(h, value)
}

// place bins one place, or keeps it until the resolution is known.
func (c *h3Cells) place(lat, lon, value float64) {
	if c.res < 0 {
		c.places = append(c.places, h3Place{lat, lon, value})
		return
	}
	if h := h3.Cell(lat, lon, c.res); h != 0 {
		c.add(h, value)
	}
}

func (c *h3Cells) add(h uint64, value float64) {
	if s, ok := c.sums[h]; ok {
		s.value = refold(c.fold, s.value, value)
		s.count++
		return
	}
	c.sums[h] = &hexSum{value: value, count: 1}
}

// shapes are the cells as shaded marks, in order of their ids, the box grown
// to hold them. points is the box of the places, which binned cells are sized
// against.
func (c *h3Cells) shapes(points, box *Bounds) []Shape {
	if c.binned {
		c.sizeTo(points)
	}
	ids := make([]uint64, 0, len(c.sums))
	for h := range c.sums {
		ids = append(ids, h)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]Shape, 0, len(ids))
	for _, h := range ids {
		s := c.sums[h]
		label := h3.String(h)
		if c.binned {
			label = places(s.count)
		}
		out = append(out, Shape{Label: label, Path: h3Path(h, box), Value: s.value, Formatted: compact(s.value)})
	}
	return out
}

// cellID reads a cell id the way warehouses store one: hex text, or the
// integer itself. Not a float — a float cannot hold sixty-four bits, and a
// cell id rounded is some other cell's.
func cellID(v any) (uint64, bool) {
	switch t := v.(type) {
	case string:
		return h3.Parse(strings.TrimSpace(t))
	case []byte:
		return h3.Parse(strings.TrimSpace(string(t)))
	case int64:
		return uint64(t), h3.Valid(uint64(t))
	case uint64:
		return t, h3.Valid(t)
	}
	return 0, false
}

/*
h3Fit is the resolution at which about DefaultHexes cells span the places, as
a hexbin sizes itself: the one whose cells are nearest that wide, across the
places' longer side, measured on the ground at their middle.
*/
func h3Fit(b *Bounds) int {
	if b.empty() {
		return h3Unsized
	}
	mid := latitude((b.MinY + b.MaxY) / 2)
	wide := (b.MaxX - b.MinX) * earthKm * math.Cos(mid*math.Pi/180)
	tall := math.Abs(latitude(b.MinY)-latitude(b.MaxY)) * earthKm / 360
	want := max(wide, tall) / DefaultHexes
	if !(want > 0) {
		return h3Unsized
	}
	best, gap := h3Unsized, math.Inf(1)
	for res := 0; res <= h3.MaxResolution; res++ {
		if d := math.Abs(math.Log(math.Sqrt(3) * h3.EdgeKm(res) / want)); d < gap {
			best, gap = res, d
		}
	}
	return best
}

// h3Path is a cell as a closed SVG subpath in world units. Its longitudes are
// kept on its first corner's side of the antimeridian, so a cell across it is
// a cell at the edge of the world rather than a band across all of it.
func h3Path(h uint64, box *Bounds) string {
	var b strings.Builder
	var lon0 float64
	for i, p := range h3.Boundary(h) {
		lon := p[1]
		if i == 0 {
			lon0 = lon
			b.WriteByte('M')
		} else {
			lon += 360 * math.Round((lon0-lon)/360)
			b.WriteByte('L')
		}
		_, y := project(0, p[0])
		x := (lon + 180) / 360
		box.add(clamp01(x), y)
		b.WriteString(coord(x))
		b.WriteByte(' ')
		b.WriteString(coord(y))
	}
	b.WriteByte('Z')
	return b.String()
}
