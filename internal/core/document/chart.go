package document

import "fmt"

// Chart is one chart printed on a document.
//
// Carries drawn marks rather than data — see Mark. What is left here is the
// text a reader needs around them: the title, the scale down the side, and the
// key that says which colour is which.
type Chart struct {
	Title string `json:"title"`
	// Kind is the chart type it came from. The template draws marks and does
	// not branch on this; it is here so a reader of the JSON — and anyone
	// debugging a wrong-looking page — can tell what it was meant to be.
	Kind  string `json:"kind"`
	Marks []Mark `json:"marks"`
	// Ticks label the vertical scale, bottom to top, where the chart has one;
	// the template rules a faint line across the box at each.
	Ticks []Tick `json:"ticks,omitempty"`
	// XTicks label the horizontal one under the box, left to right: the
	// categories of a line or a column chart, or a scatter's measure. A PDF
	// without them was a line nobody could say the months of.
	XTicks []Tick `json:"xTicks,omitempty"`
	// Ticks2 label a second scale down the right, for a measure that asked for
	// its own, set in Tone2 — its track's colour — as the screen sets it.
	Ticks2 []Tick `json:"ticks2,omitempty"`
	Tone2  string `json:"tone2,omitempty"`
	// Keys name the colours, for the charts that use more than one meaning.
	Keys []Key `json:"keys,omitempty"`
	// Square asks for a box as tall as it is wide.
	//
	// A pie drawn in a box the width of the page is an ellipse, and an ellipse
	// encodes a direction the data does not have — the eye reads the wide axis
	// as more. The charts whose geometry is circular say so here rather than
	// every renderer knowing which types those are.
	Square bool `json:"square,omitempty"`
	// Aspect asks for a box this many times as wide as it is tall, for a
	// chart whose geometry is a place. A map drawn into whatever box the page
	// has left is a country stretched sideways, and a reader measuring a
	// distance on it would be measuring the stretch.
	Aspect float64 `json:"aspect,omitempty"`
	// Height asks for a box this many millimetres tall. A chart laid out in
	// rows — bars, a funnel's stages, a heatmap's rows — asks for what its
	// rows need, so a row is a row's height whether there are two or twenty:
	// two bars stretched to the box every chart used to get were a centimetre
	// thick each. A map or a treemap asks for the room its shapes need, and a
	// map keeps its proportions within it.
	Height float64 `json:"height,omitempty"`
	// Note replaces the drawing when there is nothing to draw — an empty
	// result, or a chart type this renderer does not print. A page that simply
	// omits a block looks like a report that was never written that way.
	Note string `json:"note,omitempty"`
}

// Tick is one label on a printed scale.
type Tick struct {
	// At is where it sits, 0 at the bottom of the box and 1 at the top.
	At    float64 `json:"at"`
	Label string  `json:"label"`
}

// Key is one entry of a printed legend.
type Key struct {
	Tone  string `json:"tone"`
	Label string `json:"label"`
	// Shape is the glyph the key's marks are drawn in, as Mark.Shape.
	Shape string `json:"shape,omitempty"`
}

// validate reports what would typeset into a wrong chart rather than a failed
// one.
func (c Chart) validate() error {
	if c.Title == "" {
		return fmt.Errorf("%w: a chart has no title", ErrInvalid)
	}
	for _, m := range c.Marks {
		switch m.Kind {
		case RectMark, DotMark:
		case TextMark:
			if m.Label == "" {
				return fmt.Errorf("%w: chart %q has a text mark with nothing to say", ErrInvalid, c.Title)
			}
		case LineMark, PolyMark:
			if len(m.Points) < 2 {
				return fmt.Errorf("%w: chart %q has a %s of %d points",
					ErrInvalid, c.Title, m.Kind, len(m.Points))
			}
		default:
			return fmt.Errorf("%w: chart %q has a %q mark, which nothing draws",
				ErrInvalid, c.Title, m.Kind)
		}
	}
	return nil
}
