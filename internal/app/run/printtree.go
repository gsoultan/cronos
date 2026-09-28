package run

import (
	"unicode/utf8"

	"github.com/gsoultan/cronos/internal/core/document"
)

const (
	// treeHeight is a printed treemap's box, in millimetres: the screen's
	// proportions would make it a page.
	treeHeight = 64.0
	// treeHead is the strip across the top of a group's frame its name is
	// set in, in millimetres, and a frame shorter than two of them has none.
	treeHead = 4.6
)

// treemap places the rectangles the server laid out as the screen places
// them: each group a frame with its name across the top and its leaves under
// it, each leaf named and figured where it has room. A printed treemap was
// its leaves alone, coloured by a group nothing on the page named.
func treemap(c *document.Chart, rects []Rect) {
	c.Height = treeHeight
	nested := hasLeaves(rects)
	frames := map[string]Rect{}
	for _, r := range rects {
		if nested && r.Depth == 0 {
			frames[r.Label] = r
			c.Marks = append(c.Marks, frame(r)...)
		}
	}
	for _, r := range rects {
		if nested && r.Depth == 0 {
			continue
		}
		if f, ok := frames[r.Group]; ok {
			r = under(r, f)
		}
		c.Marks = append(c.Marks, leaf(r)...)
	}
}

// frame is a group's ground and its name across the top.
func frame(f Rect) []document.Mark {
	out := []document.Mark{{Kind: document.RectMark, X: f.X, Y: f.Y, W: f.W * 0.996, H: f.H * 0.994,
		Tone: "frame", Label: f.Label, Value: f.Formatted}}
	if head := headOf(f); head > 0 {
		if label := cut(f.Label, runesIn(f.W-0.01)); utf8.RuneCountInString(label) >= 3 {
			out = append(out, text(f.X+0.005, f.Y+head/2, label, "ink", document.StartAnchor, true))
		}
	}
	return out
}

// headOf is the strip a frame keeps for its name, as a share of the box.
func headOf(f Rect) float64 {
	if f.H*treeHeight < treeHead*2 {
		return 0
	}
	return treeHead / treeHeight
}

// under fits a leaf into what its frame has left under its name.
func under(r, f Rect) Rect {
	head := headOf(f)
	k := (f.H - head) / f.H
	r.Y = f.Y + head + (r.Y-f.Y)*k
	r.H *= k
	return r
}

// leaf is one rectangle, inset a hair so neighbours read as two, and its name
// and figure at its top left when it has the room.
func leaf(r Rect) []document.Mark {
	colour := toneFor(r)
	out := []document.Mark{{Kind: document.RectMark, X: r.X + 0.002, Y: r.Y + 0.004,
		W: maxOf(r.W-0.004, 0.001), H: maxOf(r.H-0.008, 0.001), Tone: colour, Label: r.Label, Value: r.Formatted}}
	tall, room := r.H*treeHeight, runesIn(r.W-0.014)
	name := cut(r.Label, room)
	if tall < 7 || utf8.RuneCountInString(name) < 3 {
		return out
	}
	x := r.X + 0.008
	out = append(out, text(x, r.Y+4/treeHeight, name, inkOn(colour), document.StartAnchor, true))
	if tall >= 11 && utf8.RuneCountInString(r.Formatted) <= room {
		out = append(out, text(x, r.Y+7.6/treeHeight, r.Formatted, inkOn(colour), document.StartAnchor, false))
	}
	return out
}

func hasLeaves(rects []Rect) bool {
	for _, r := range rects {
		if r.Depth == 1 {
			return true
		}
	}
	return false
}

func toneFor(r Rect) string {
	if r.Depth == 1 {
		return tone(r.Slot)
	}
	return step(r.Slot)
}
