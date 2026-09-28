package run

import (
	"context"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// mapAt is where a map block sits in a render, and what it is rendered with:
// the report's filters, and the map's own — less the one it is the control
// for, see control.
type mapAt struct {
	out     definition.Output
	i       int
	params  map[string]any
	filters query.Filters
	own     query.Filters
	values  map[string]query.FilterValue
}

/*
mapped finishes a map block once its own query has been read: the whole of it
where that query was cut, the datasets drawn over it, and — for a browser —
the filters a reader can set from it and the basemap under it.

Only a browser draws a basemap. A PDF prints the data without one, so resolving
tiles for it would be a Google session minted per statement of a
five-thousand-recipient burst, for nothing; and a page cannot be clicked.
*/
func (s *Service) mapped(ctx context.Context, r definition.Report, at mapAt, m *GeoMap,
	pr principal.Principal) (*GeoMap, error) {

	blk := at.out.Layout[at.i]
	paper := at.out.Renderer != definition.Interactive
	m, err := s.whole(ctx, r, blk, m, Detail{Output: at.out.Name, Block: at.i}, paper, at.params, at.own, pr)
	if err != nil {
		return nil, err
	}
	if err := s.overlay(ctx, r, blk, m, at, paper, pr); err != nil {
		return nil, err
	}
	if paper {
		return m, nil
	}
	if ds, err := s.datasets.Dataset(ctx, blk.DatasetFor(r.Dataset)); err == nil {
		setsFilters(m, r, blk, ds, at.values)
	}
	if blk.Map.Basemap != nil {
		m.Tiles, m.Note = s.tiles(ctx, *blk.Map.Basemap, m.Bounds)
	}
	return m, nil
}

// whole is a map's own query's answer or, where that was cut, the whole of it
// gathered — see large.
func (s *Service) whole(ctx context.Context, r definition.Report, blk definition.Block, m *GeoMap,
	detail Detail, paper bool, params map[string]any, f query.Filters,
	pr principal.Principal) (*GeoMap, error) {

	switch {
	case !m.cut:
		return m, nil
	case blk.Map.Time != nil:
		// Gathered, a map would lose its periods; it plays what it read, and
		// says under itself that that is not all of it.
		return m, nil
	case pointed(blk.Map):
		return s.large(ctx, r, blk, detail, paper, params, f, pr)
	case blk.Map.Draws(definition.H3Layer):
		// Cells a warehouse indexed, more of them than a map's query holds.
		return s.largeCells(ctx, r, blk, params, f, pr)
	}
	return m, nil
}

/*
overlay draws each dataset listed over the map, each through the same path a
map takes — its own query, gathered where that is cut — and grows the map's box
to hold whichever of them have anything to draw.
*/
func (s *Service) overlay(ctx context.Context, r definition.Report, blk definition.Block, m *GeoMap,
	at mapAt, paper bool, pr principal.Principal) error {

	var box *Bounds
	if marked(m) {
		b := m.Bounds
		box = &b
	}
	for j, ov := range blk.OverlaysFor(r.Dataset) {
		b, err := s.block(ctx, r, ov, at.params, at.filters, pr)
		if err != nil {
			return err
		}
		detail := Detail{Output: at.out.Name, Block: at.i, Overlay: j + 1}
		g, err := s.whole(ctx, r, ov, b.Map, detail, paper, at.params, at.filters, pr)
		if err != nil {
			return err
		}
		g.Title = ov.Heading()
		m.Overlays = append(m.Overlays, g)
		if marked(g) {
			box = around(box, g.Bounds)
		}
	}
	if box != nil {
		m.Bounds = *box
	}
	return nil
}

// marked reports whether a map has anything to draw — its box is otherwise
// the whole world, which is no box to fit an overlay into.
func marked(m *GeoMap) bool {
	n := len(m.Shapes) + len(m.Markers) + len(m.Arcs) + len(m.Lines) + len(m.Hexes)
	return n > 0 || (m.Cells != nil && len(m.Cells.X) > 0)
}

// around is the box holding both.
func around(box *Bounds, b Bounds) *Bounds {
	if box == nil {
		return &b
	}
	return &Bounds{
		MinX: min(box.MinX, b.MinX), MinY: min(box.MinY, b.MinY),
		MaxX: max(box.MaxX, b.MaxX), MaxY: max(box.MaxY, b.MaxY),
	}
}
