package run

import (
	"unicode/utf8"

	"github.com/gsoultan/cronos/internal/core/document"
)

// referencesOn draws the block's reference lines over its marks, as the
// browser does: a dashed rule across the box at the value, named with its
// figure where the name covers none of the figures the marks carry — the
// right end above the rule, else the left, else under it.
func referencesOn(c *document.Chart, b Block) {
	if b.YAxis == nil {
		return
	}
	taken := wordsOf(c.Marks)
	for _, r := range b.References {
		if r.Value < b.YAxis.Min || r.Value > b.YAxis.Max {
			continue
		}
		y := 1 - norm(r.Value, b.YAxis.Min, b.YAxis.Max)
		text := r.Formatted
		if r.Label != "" {
			text = r.Label + " " + text
		}
		spot := nameSpot(y, widthOf(text), taken)
		taken = append(taken, spot)
		mark := document.Mark{Kind: document.TextMark, X: spot.x2, Y: (spot.y1 + spot.y2) / 2,
			Label: text, Anchor: "end", Strong: true}
		if spot.x1 < 0.5 {
			mark.X, mark.Anchor = spot.x1, "start"
		}
		// On a plate of paper, so a bar under the name does not take it.
		c.Marks = append(c.Marks,
			document.Mark{Kind: document.LineMark, Points: [][2]float64{{0, y}, {1, y}},
				Tone: "ink", Stroke: 0.8, Dash: true},
			document.Mark{Kind: document.RectMark, X: spot.x1 - 0.004, Y: spot.y1,
				W: spot.x2 - spot.x1 + 0.008, H: spot.y2 - spot.y1, Tone: "paper"},
			mark)
	}
}

// plate is a rectangle of the chart's box, in its fractions.
type plate struct{ x1, x2, y1, y2 float64 }

func (a plate) overlaps(b plate) bool {
	return a.x1 < b.x2 && b.x1 < a.x2 && a.y1 < b.y2 && b.y1 < a.y2
}

// wordHeight is a printed figure's height as a share of a chart's box, and
// charWidth one character's width: the template sets figures at about 6.5pt
// in a box about 170mm by 60mm. Close enough to tell whether two collide.
const (
	wordHeight = 0.06
	charWidth  = 0.0075
)

func widthOf(text string) float64 { return float64(utf8.RuneCountInString(text)) * charWidth }

// wordsOf is where each text mark already printed sits.
func wordsOf(marks []document.Mark) []plate {
	var out []plate
	for _, m := range marks {
		if m.Kind != document.TextMark {
			continue
		}
		w := widthOf(m.Label)
		x1 := m.X - w/2
		switch m.Anchor {
		case "start":
			x1 = m.X
		case "end":
			x1 = m.X - w
		}
		out = append(out, plate{x1, x1 + w, m.Y - wordHeight/2, m.Y + wordHeight/2})
	}
	return out
}

// nameSpot is where a rule at y is named: the first of the four places that
// covers no word, or the one covering fewest.
func nameSpot(y, w float64, taken []plate) plate {
	var spots []plate
	for _, y1 := range []float64{y - wordHeight - 0.01, y + 0.01} {
		if y1 < -0.02 || y1+wordHeight > 1 {
			continue
		}
		for _, x1 := range []float64{1 - w, 0} {
			spots = append(spots, plate{x1, x1 + w, y1, y1 + wordHeight})
		}
	}
	if len(spots) == 0 {
		return plate{1 - w, 1, y - wordHeight/2, y + wordHeight/2}
	}
	covered := func(s plate) int {
		n := 0
		for _, t := range taken {
			if s.overlaps(t) {
				n++
			}
		}
		return n
	}
	best := spots[0]
	for _, s := range spots[1:] {
		if covered(s) < covered(best) {
			best = s
		}
	}
	return best
}
