package document

// Mark is one shape on a printed chart, placed in a unit box.
//
// Four primitives rather than a shape per chart type. A typesetter is not a
// charting library: giving the template a case per chart would mean fourteen
// drawing routines in a language chosen for typesetting, kept in step with
// fourteen more in the browser. Normalising to rectangles, lines, polygons and
// dots on this side means the template draws *marks*, and a new chart type is
// a new arrangement of the same four.
//
// It is the same decision as the map's projection: the engine that knows the
// numbers works out where the shapes go, once, for every renderer.
type Mark struct {
	// Kind is rect, line, poly or dot.
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
	// Label and Value are drawn beside the mark where there is room for them.
	Label string `json:"label,omitempty"`
	Value string `json:"value,omitempty"`
}

// Mark kinds.
const (
	RectMark = "rect"
	LineMark = "line"
	PolyMark = "poly"
	DotMark  = "dot"
)
