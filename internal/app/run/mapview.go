package run

import (
	"context"
	"fmt"
	"math"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// maxViewPixels is the largest stage a view may say it is drawn in. The grid
// is sized from it, so a caller claiming a screen a million pixels wide would
// otherwise be asking for cells a million pixels' worth fine.
const maxViewPixels = 8192

/*
MapView is one view of a large map: its places in the part of the world the
reader has in view, gathered at the grid that view's scale wants — until, zoomed
in far enough, every cell is a single place and the view is the places
themselves.

Everything a render applies is applied again: the report's parameters and its
pinned ones, the filters, the reader's own scope. A view is a render of less of
the world, and the viewer sends nothing here it could not have sent to the
render.
*/
func (s *Service) MapView(ctx context.Context, r definition.Report, req ViewRequest,
	pr principal.Principal) (*GeoMap, error) {

	out, err := pick(r, req.Output)
	if err != nil {
		return nil, err
	}
	blk, err := viewedBlock(out, req.Block, req.Overlay, r.Dataset)
	if err != nil {
		return nil, err
	}
	view, err := req.checked()
	if err != nil {
		return nil, err
	}
	params, err := applyOverrides(r, req.Params)
	if err != nil {
		return nil, err
	}
	filters := query.Filters{Defs: r.Filters, Values: req.Filters}
	if req.Overlay == 0 {
		filters = s.control(ctx, r, out, blk, filters)
	}
	q, err := s.bigMapFor(ctx, r, blk, params, filters, pr)
	if err != nil {
		return nil, err
	}
	cells := gridFor(view, req.Width, req.Height, q.keyed())
	g := query.MapGrid{Cells: cells, Box: degrees(view, 1/float64(cells))}

	m := emptyMap(blk)
	m.Bounds = view
	if err := q.view(ctx, m, g, slotsOf(capped(req.Categories))); err != nil {
		return nil, err
	}
	return m, nil
}

// viewedBlock is the map a view names — a block, or a dataset drawn over one —
// and only a map with places.
func viewedBlock(out definition.Output, at, overlay int, reportDefault string) (definition.Block, error) {
	if at < 0 || at >= len(out.Layout) {
		return definition.Block{}, fmt.Errorf("%w: output %q has no block %d", ErrNotAMap, out.Name, at)
	}
	blk := out.Layout[at]
	if overlay > 0 {
		ovs := blk.OverlaysFor(reportDefault)
		if overlay > len(ovs) {
			return definition.Block{}, fmt.Errorf("%w: block %d of output %q has no overlay %d",
				ErrNotAMap, at, out.Name, overlay)
		}
		blk = ovs[overlay-1]
	}
	if blk.Kind != definition.ChartBlock || blk.Map == nil || !pointed(blk.Map) || overlay < 0 {
		return definition.Block{}, fmt.Errorf("%w: block %d of output %q", ErrNotAMap, at, out.Name)
	}
	return blk, nil
}

// checked is the view, clamped to the world, once its numbers are ones a
// query can be narrowed by.
func (req ViewRequest) checked() (Bounds, error) {
	v := req.View
	for _, n := range []float64{v.MinX, v.MinY, v.MaxX, v.MaxY} {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return Bounds{}, fmt.Errorf("%w: a view that is not a number", ErrNotAMap)
		}
	}
	if req.Width < 1 || req.Height < 1 || req.Width > maxViewPixels || req.Height > maxViewPixels {
		return Bounds{}, fmt.Errorf("%w: a view %d by %d pixels", ErrNotAMap, req.Width, req.Height)
	}
	v = Bounds{MinX: clamp01(v.MinX), MinY: clamp01(v.MinY), MaxX: clamp01(v.MaxX), MaxY: clamp01(v.MaxY)}
	if !(v.MaxX > v.MinX) || !(v.MaxY > v.MinY) {
		return Bounds{}, fmt.Errorf("%w: a view with no area", ErrNotAMap)
	}
	return v, nil
}

// degrees is a view in latitude and longitude, grown by a margin — a cell, so
// a place just off the edge still pulls its cell's middle to where it is.
func degrees(v Bounds, margin float64) *query.GeoBox {
	x0, x1 := clamp01(v.MinX-margin), clamp01(v.MaxX+margin)
	y0, y1 := clamp01(v.MinY-margin), clamp01(v.MaxY+margin)
	// y grows southward, so the top of the view is its northern edge.
	box := &query.GeoBox{
		North: latitude(y0), South: latitude(y1),
		West: x0*360 - 180, East: x1*360 - 180,
	}
	// A view touching the edge of the world takes in what lies past it:
	// project clamps a place beyond Web Mercator's limit onto the edge, and
	// the overview drew it there.
	if y0 == 0 {
		box.North = 90
	}
	if y1 == 1 {
		box.South = -90
	}
	return box
}

// capped keeps a viewer's list of categories to what a map names.
func capped(names []string) []string {
	if len(names) > categoriesKept {
		names = names[:categoriesKept]
	}
	return names
}
