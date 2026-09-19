package run

// Rect is one rectangle of a treemap, already laid out.
//
// The layout happens on the server for the same reason the map's projection
// does: squarifying is an algorithm with a right answer, and running it in
// every viewer means shipping it to every one of an ISV's end users and
// getting a different answer in the PDF.
//
// Coordinates are fractions of the whole map, so a viewer scales them to
// whatever width it has without re-laying anything out.
type Rect struct {
	Label     string  `json:"label"`
	Value     float64 `json:"value"`
	Formatted string  `json:"formatted"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	W         float64 `json:"w"`
	H         float64 `json:"h"`
	// Group is the outer rectangle this one sits in, when the treemap nests.
	Group string `json:"group,omitempty"`
	// Slot is the colour: the group's categorical slot when nested, and the
	// ordinal ramp step when not.
	Slot int `json:"slot"`
	// Depth is 0 for a group's own frame and 1 for a leaf inside it.
	Depth int `json:"depth"`
}
