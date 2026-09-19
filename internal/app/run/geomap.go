package run

// GeoMap is everything a map block draws.
//
// One payload with a list per layer, rather than a payload per layer, because
// a map is one query: the rows that shade a region are the rows that place its
// dots, and splitting them would mean a viewer reconciling two lists that came
// from the same GROUP BY.
type GeoMap struct {
	// Bounds is the world-unit box to fit, already padded.
	Bounds Bounds `json:"bounds"`
	// Layers is what to draw, bottom to top. The viewer draws what it knows
	// and ignores the rest, which is how a map gains a layer without every
	// pinned copy of the bundle breaking.
	Layers []string `json:"layers"`

	// Never omitempty, for the reason Block.Series is not: a client should
	// never have to tell an absent list from an empty one, and the emptiest
	// report is the one most likely to be the first to arrive.
	Shapes  []Shape  `json:"shapes"`
	Markers []Marker `json:"markers"`
	Arcs    []Arc    `json:"arcs"`
	Legend  []Legend `json:"legend"`

	// Tiles is the basemap, when the author asked for one.
	Tiles *Tiles `json:"tiles,omitempty"`
}
