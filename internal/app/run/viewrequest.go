package run

// ViewRequest is a viewer asking for the part of a large map it has in view.
//
// The report's own request comes with it — output, parameters, filters — and
// is applied again exactly as a render applies it: a view is the same question
// asked of a smaller part of the world, not a different question with its own
// rules.
type ViewRequest struct {
	Request
	// Block is the map's place in the output's layout, and Overlay which
	// dataset over it — zero for the map itself — as Detail named them.
	Block, Overlay int
	// View is what the reader sees, in world units, and Width and Height the
	// CSS pixels it is drawn in.
	View          Bounds
	Width, Height int
	// Categories are the map's own, as Detail sent them — what every view of
	// it is coloured by.
	Categories []string
}
