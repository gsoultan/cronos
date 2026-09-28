package run

import (
	"context"
	"fmt"
	"math"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
A map with more places than one payload holds.

A map's own query reads a row per place and stops at query.ChartLimit. When it
stops, the map is asked again, differently: where its places are, then how many
fall in each cell of a grid fine enough for the depth it opens at — and as a
reader zooms in, the same question about the part in view, a finer grid each
time, until the cells are one place each. The database does the gathering over
every row; what crosses the wire is bounded by the screen, not by the table.
*/

const (
	// openWidth and openHeight are the size a large map is gathered for before
	// any viewer has said how big it is: a laptop's panel, about. The first
	// view it asks for is gathered at its own size.
	openWidth  = 1024
	openHeight = 640
	// printWidth and printHeight are the same for a page, which cannot zoom:
	// a few thousand cells at most, because every one is a mark a typesetter
	// places and a reader can only see so many of on paper.
	printWidth  = 360
	printHeight = 225
	// cellPixels is the smallest a cell is drawn, in CSS pixels: small enough
	// that a density reads as texture, large enough that a full screen of
	// them stays under query.MaxCells.
	cellPixels = 4
	// categoriesKept is how many categories a large map names. The palette
	// colours fewer; the rest are counted into its last swatch.
	categoriesKept = 64
)

// bigMap is one large map block and everything a question about it needs.
type bigMap struct {
	blk     definition.Block
	ds      definition.Dataset
	engine  Engine
	params  map[string]any
	filters query.Filters
	pr      principal.Principal
}

// bigMapFor resolves the block's dataset and engine.
func (s *Service) bigMapFor(ctx context.Context, r definition.Report, blk definition.Block,
	params map[string]any, filters query.Filters, pr principal.Principal) (*bigMap, error) {

	ds, err := s.datasets.Dataset(ctx, blk.DatasetFor(r.Dataset))
	if err != nil {
		return nil, err
	}
	engine, err := s.engines.Engine(ctx, ds)
	if err != nil {
		return nil, err
	}
	return &bigMap{blk: blk, ds: ds, engine: engine, params: params, filters: filters, pr: pr}, nil
}

// large draws a map whose own query was cut: the whole of it, gathered.
func (s *Service) large(ctx context.Context, r definition.Report, out definition.Output, at int,
	params map[string]any, filters query.Filters, pr principal.Principal) (*GeoMap, error) {

	blk := out.Layout[at]
	q, err := s.bigMapFor(ctx, r, blk, params, filters, pr)
	if err != nil {
		return nil, err
	}
	sv, err := q.survey(ctx)
	if err != nil {
		return nil, err
	}
	m := emptyMap(blk)
	if sv.box.empty() {
		return m, nil
	}
	// The shapes first: an outline reaches past the places inside it, and
	// the box the map opens on has to hold both.
	if err := q.shapes(ctx, m, sv); err != nil {
		return nil, err
	}
	m.Bounds = sv.box.padded()
	slot, err := q.colours(ctx, m)
	if err != nil {
		return nil, err
	}
	w, h := openWidth, openHeight
	if out.Renderer != definition.Interactive {
		w, h = printWidth, printHeight
	}
	grid := query.MapGrid{Cells: gridFor(m.Bounds, w, h, q.keyed())}
	if err := q.view(ctx, m, grid, slot); err != nil {
		return nil, err
	}
	shade(m)
	// Only a map whose places are drawn as places has more to show up close:
	// a hexagon is the same hexagon at every zoom, and asking again would be
	// a query per pan for the answer already on screen.
	if markersDrawn(blk.Map) || blk.Map.Draws(definition.FlowLayer) {
		m.Detail = &Detail{Output: out.Name, Block: at, Categories: m.categories}
	}
	return m, nil
}

// shapes reads what does not change as a reader zooms: regions and lines at
// their own grain, and hexagons of a fixed width. Each grows the box.
func (q *bigMap) shapes(ctx context.Context, m *GeoMap, sv survey) error {
	spec := q.blk.Map
	if spec.Draws(definition.PolygonLayer) || spec.Draws(definition.LineLayer) {
		if err := q.regions(ctx, m, sv.box); err != nil {
			return err
		}
	}
	if spec.Draws(definition.HexbinLayer) {
		// Sized from the places alone, as the small path sizes them — and
		// before anything else has grown the box.
		radius := hexRadius(spec.HexKm, sv.points)
		if err := q.hexes(ctx, m, radius, sv.box); err != nil {
			return err
		}
	}
	return nil
}

// colours names the map's categories, most common first, and gives each its
// swatch. Empty for a map that colours nothing by category.
func (q *bigMap) colours(ctx context.Context, m *GeoMap) (map[string]int, error) {
	if q.blk.Series.Field == "" {
		return map[string]int{}, nil
	}
	names, err := q.categories(ctx)
	if err != nil {
		return nil, err
	}
	slot := slotsOf(names)
	m.categories = names
	m.Keys = categoryKeys(names, slot)
	return slot, nil
}

// view gathers the points and flows of one view: the whole map as it opens,
// or the part a reader has zoomed to.
func (q *bigMap) view(ctx context.Context, m *GeoMap, g query.MapGrid, slot map[string]int) error {
	if markersDrawn(q.blk.Map) {
		c, err := q.cells(ctx, g, slot)
		if err != nil {
			return err
		}
		m.Cells = c
		if m.Places == 0 {
			// Unless the hexagons already counted the same places.
			for _, n := range c.N {
				m.Places += n
			}
		}
		if c.cut {
			m.Partial = cutAt(group(query.MaxCells, 0) + " cells of places")
		}
	}
	if q.blk.Map.Draws(definition.FlowLayer) {
		return q.flows(ctx, m, g, slot)
	}
	return nil
}

// emptyMap is a map with its layers named and every list present.
func emptyMap(blk definition.Block) *GeoMap {
	return &GeoMap{
		Layers: layerNames(blk.Map), Shapes: []Shape{}, Markers: []Marker{},
		Arcs: []Arc{}, Legend: []Legend{},
	}
}

/*
gridFor is how many cells span the world for a view drawn at w by h pixels: a
power of two, so the grid is the same grid wherever the reader pans, and the
largest one whose cells are still cellPixels wide on screen — or wider, on a
screen large enough that cells that small would be more than query.MaxCells.
*/
func gridFor(view Bounds, w, h int, keyed bool) int {
	vw, vh := view.MaxX-view.MinX, view.MaxY-view.MinY
	if !(vw > 0) || !(vh > 0) || w <= 0 || h <= 0 {
		return 1
	}
	// Pixels per world unit, fitting the whole view into the stage the way
	// the viewer fits it.
	scale := math.Min(float64(w)/vw, float64(h)/vh)
	px := math.Max(cellPixels, math.Sqrt(float64(w)*float64(h)/query.MaxCells))
	if keyed {
		// A cell per category, so a cell of mixed places is several rows: a
		// grid twice as coarse keeps a map of five carriers inside the same
		// bound as a map of one.
		px *= 2
	}
	cells := math.Exp2(math.Floor(math.Log2(scale / px)))
	return int(math.Min(math.Max(cells, 1), 1<<30))
}

// keyed reports whether the block colours its points by category.
func (q *bigMap) keyed() bool { return q.blk.Series.Field != "" }

// pointed reports whether any of a map's layers reads a place per row — the
// only kind of map that can have more rows than it draws and be gathered.
// A map of outlines alone is cut where it is cut, and says so.
func pointed(m *definition.MapSpec) bool {
	for _, l := range m.Resolved() {
		if l.Points() {
			return true
		}
	}
	return false
}

// slotsOf colours categories in the order given — most common first — with
// every one past the palette sharing its last swatch.
func slotsOf(names []string) map[string]int {
	out := make(map[string]int, len(names))
	for i, n := range names {
		out[n] = min(i, PlotSlots-1)
	}
	return out
}

// cutAt is the sentence under a large map one of whose questions came back
// with more than it may draw.
func cutAt(what string) string {
	return fmt.Sprintf("This map draws the first %s — zoom in or narrow the filters to see the rest.", what)
}
