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

	// Cells are the places of a map too large to send one by one, gathered
	// into a grid fine enough for the depth it opens at. Nil for a map that
	// fits, whose places are Markers as they always were.
	Cells *Cells `json:"cells,omitempty"`
	// Places is how many places a large map holds — the number no list in
	// this payload is long enough to count.
	Places int `json:"places,omitempty"`
	// Detail is how a viewer asks for the part of a large map in view, at
	// the depth it is looking at. Nil for a map that fits.
	Detail *Detail `json:"detail,omitempty"`

	// cut is whether the map's own query had more rows than a map holds,
	// which is the moment it is asked again as a large one.
	cut bool
	// categories are a large map's, most common first — the order its
	// colours were given in, which every view of it is coloured by.
	categories []string
}
