package run

/*
MapArea is the area filter a map can set from its view: the one bound to the
map's own coordinates. "Filter to this view" sets it to the box the reader has
zoomed to, and every block it reaches narrows to that part of the world.

Op and Values are what it holds now — a box as south, west, north, east, or a
distance around a place — so the map can draw the area it is filtered to.
*/
type MapArea struct {
	Filter string    `json:"filter"`
	Op     string    `json:"op,omitempty"`
	Values []float64 `json:"values,omitempty"`
}
