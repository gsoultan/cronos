package run

// Bounds is the Web Mercator world-unit box a map's data occupies.
//
// Sent so the viewer can fit the data without knowing any geography. It is
// also what lets a basemap work: world units and tile coordinates are the same
// space, so a viewer converts this box to a zoom and a tile range with
// arithmetic rather than with a projection library.
type Bounds struct {
	MinX float64 `json:"minX"`
	MinY float64 `json:"minY"`
	MaxX float64 `json:"maxX"`
	MaxY float64 `json:"maxY"`
}

// wholeWorld is the box a map with nothing in it falls back to.
var wholeWorld = Bounds{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}

// newBounds starts an inverted box, so the first point sets all four sides.
func newBounds() *Bounds {
	return &Bounds{MinX: 1, MinY: 1, MaxX: 0, MaxY: 0}
}

// add grows the box to contain (x, y).
func (b *Bounds) add(x, y float64) {
	b.MinX = min(b.MinX, x)
	b.MinY = min(b.MinY, y)
	b.MaxX = max(b.MaxX, x)
	b.MaxY = max(b.MaxY, y)
}

// empty reports whether nothing was ever added.
func (b *Bounds) empty() bool { return b.MinX > b.MaxX || b.MinY > b.MaxY }

// padded is the box to actually send: grown by a margin so marks on the edge
// are not half-clipped, and never collapsed to a line.
//
// A single point has no extent at all, and a viewBox of zero width is a map
// that renders as nothing — the failure looks identical to no data, which is
// the reason it is handled here rather than left to each viewer.
func (b *Bounds) padded() Bounds {
	if b.empty() {
		return wholeWorld
	}
	out := *b
	// One point, or several at one place, has no extent to pad — 4% of zero is
	// zero, and the viewBox stays a dot. Give it a span first: 1/256 of the
	// world is roughly a city, which is the scale somebody looking at a single
	// marker meant.
	if span := max(b.MaxX-b.MinX, b.MaxY-b.MinY); span < 1.0/256 {
		grow := (1.0/256 - span) / 2
		out.MinX, out.MaxX = b.MinX-grow, b.MaxX+grow
		out.MinY, out.MaxY = b.MinY-grow, b.MaxY+grow
	}

	// Padded so a mark sitting on the edge is not half-clipped by the viewBox.
	pad := max(out.MaxX-out.MinX, out.MaxY-out.MinY) * 0.04
	return Bounds{
		MinX: max(0, out.MinX-pad), MinY: max(0, out.MinY-pad),
		MaxX: min(1, out.MaxX+pad), MaxY: min(1, out.MaxY+pad),
	}
}
