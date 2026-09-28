package document

// Mark is one shape on a printed chart, placed in a unit box.
//
// Five primitives rather than a shape per chart type. A typesetter is not a
// charting library: giving the template a case per chart would mean fourteen
// drawing routines in a language chosen for typesetting, kept in step with
// fourteen more in the browser. Normalising to rectangles, lines, polygons,
// dots and words on this side means the template draws *marks*, and a new
// chart type is a new arrangement of the same five.
//
// It is the same decision as the map's projection: the engine that knows the
// numbers works out where the shapes go, once, for every renderer.
type Mark struct {
	// Kind is rect, line, poly, dot or text.
	Kind string `json:"kind"`
	// X, Y, W and H are fractions of the chart's box, y measured downward.
	// Used by rect and, as a centre and a radius, by dot.
	//
	// Never omitempty. Zero is a position — the left edge, the top edge — and
	// dropping the key turns a mark at the origin into one the template cannot
	// place at all. A dot on a scatter's baseline is exactly that mark.
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
	// Points are the vertices of a line or polygon, in the same fractions.
	Points [][2]float64 `json:"points,omitempty"`
	// Tone names a colour in the template's palette rather than carrying one.
	// A hex here would be the one part of the document that ignored the
	// brand's own palette, and the template is where that lives.
	Tone string `json:"tone,omitempty"`
	// Label and Value say what a shape is — the region, the place, its
	// figure — for whoever reads the document rather than its page. A text
	// mark's Label is the text it prints.
	Label string `json:"label,omitempty"`
	Value string `json:"value,omitempty"`
	// Shape is a dot's glyph — square or triangle, a circle when empty — so a
	// category reads on a page printed in grey, or by a reader who cannot
	// tell its colour from the next one's. Each at the circle's area.
	Shape string `json:"shape,omitempty"`
	// Anchor is which end of a text mark sits at X: start, end, or its middle
	// when empty; it is centred on Y either way. Strong sets it as a figure
	// rather than a name, and Size in points rather than the template's own.
	// A text mark's W, when set, is the widest it may run, as a share of the
	// box's width, and the template sets it smaller to fit.
	Anchor string  `json:"anchor,omitempty"`
	Strong bool    `json:"strong,omitempty"`
	Size   float64 `json:"size,omitempty"`
	// Stroke is a line's width in points; a full point when empty. A thread
	// between two columns is a hairline, and drawn at a line's weight it
	// reads as a series. Around a polygon it is an edge of paper, the gap
	// that keeps two slices of a pie from reading as one fill.
	Stroke float64 `json:"stroke,omitempty"`
}

// Dot shapes. A dot naming none is a circle; a key naming none is the
// rounded square every chart's key is, so a map's round key says so.
const (
	CircleShape   = "circle"
	SquareShape   = "square"
	TriangleShape = "triangle"
)

// Mark kinds.
const (
	RectMark = "rect"
	LineMark = "line"
	PolyMark = "poly"
	DotMark  = "dot"
	// TextMark is a label placed where the arithmetic put it: a bar's value
	// at its end, a category's name beside its bar, a donut's whole in its
	// middle. Its Label is the text, and its Tone ink, muted or white, or a
	// palette colour.
	TextMark = "text"
)

// Text mark anchors.
const (
	StartAnchor = "start"
	EndAnchor   = "end"
)
