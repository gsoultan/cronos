package query

// MaxCells is the most cells a large map comes back as: a full-screen map at
// four pixels a cell, about. Each is a place or a count of them, so this bounds
// what a map sends a browser however many rows are behind it.
const MaxCells = 50_000

/*
MapGrid is how a large map's points are gathered for one view.

Cells is how many cells span the world along each axis, a power of two: the
grid is then the same grid wherever the reader pans, so a cell does not change
its count as the edge of the view moves across it. Box narrows the points to
the view; nil is the whole map.
*/
type MapGrid struct {
	Cells int
	Box   *GeoBox
}
