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

	// Lines are the line layer's routes and Hexes the hexbin layer's cells.
	// Shapes as far as a viewer is concerned — a label, a path, a value and a
	// step of the ramp — kept apart because one is stroked and one is filled,
	// and a viewer telling them apart by parsing the path would be guessing.
	// Omitted when empty, unlike the four above: a viewer that predates them
	// has no use for the key, and one that knows them reads a missing list as
	// an empty one.
	Lines []Shape `json:"lines,omitempty"`
	Hexes []Shape `json:"hexes,omitempty"`
	// Keys name the colours when points are coloured by category.
	Keys []MapKey `json:"keys,omitempty"`

	// Tiles is the basemap, when the author asked for one and it could be
	// drawn.
	Tiles *Tiles `json:"tiles,omitempty"`
	// Note says why a basemap the author asked for is not under the data,
	// in words fit for whoever is reading the report. The data is drawn
	// either way: a map without its streets is a lesser map, and a map
	// withheld because a key expired is no map at all.
	Note string `json:"note,omitempty"`
	// Partial says the map was drawn from the first rows of more than it
	// could hold, and what that costs: the rest are not on it, and a total
	// folded from its points — a region's, a hexagon's — counts only those.
	// A map cut at the cap looked like the whole of its data until this.
	Partial string `json:"partial,omitempty"`
}
