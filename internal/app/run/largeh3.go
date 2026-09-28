package run

import (
	"context"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
h3Most is as many rows as a large map's h3 layer folds: places, for cells
binned from them, or a warehouse's distinct cells, for cells it indexed.

Every other layer of a large map is gathered in the database. An H3 cell has no
form in the SQL most warehouses speak, so this one layer reads rows — one pass,
keeping the cells and never the rows, and stopping here with a sentence saying
so rather than reading on for as long as the table is.
*/
const h3Most = 200_000

// h3 folds a large map's rows into its H3 cells as they stream. The survey
// has already found the places' box, so cells binned from places are sized
// before the first one arrives and none is kept.
func (q *bigMap) h3(ctx context.Context, m *GeoMap, sv survey) error {
	p, err := q.engine.Builder.BuildMapPlaces(q.ds, q.blk, q.params, q.filters, q.pr)
	if err != nil {
		return err
	}
	cells := newH3Cells(q.blk.Map, foldOf(q.blk.Y, q.ds))
	if cells.binned {
		cells.sizeTo(sv.points)
	}
	read := 0
	cut, err := q.each(ctx, p, h3Most, func(r row) error {
		read++
		if !cells.binned {
			cells.cell(r.raw(string(definition.H3Col)), r.num("value"))
			return nil
		}
		lat, lon := r.num(string(definition.LatCol)), r.num(string(definition.LonCol))
		if !r.null(string(definition.LatCol)) && finite(lon, lat) {
			cells.place(lat, lon, r.num("value"))
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.Hexes = cells.shapes(sv.points, sv.box)
	if cells.binned {
		m.Places = read
	}
	if cut {
		m.Partial = "These H3 cells count the first " + group(h3Most, 0) + " rows — index the " +
			"rows by H3 cell in the warehouse, or narrow the filters, to count them all."
	}
	return nil
}

// largeCells draws a map of cells a warehouse indexed, too many for the map's
// own query: every distinct cell, folded as it streams. There are no places
// to survey — the cells' corners are the box.
func (s *Service) largeCells(ctx context.Context, r definition.Report, blk definition.Block,
	params map[string]any, filters query.Filters, pr principal.Principal) (*GeoMap, error) {

	q, err := s.bigMapFor(ctx, r, blk, params, filters, pr)
	if err != nil {
		return nil, err
	}
	m := emptyMap(blk)
	sv := survey{box: newBounds(), points: newBounds()}
	if err := q.h3(ctx, m, sv); err != nil {
		return nil, err
	}
	if !sv.box.empty() {
		m.Bounds = sv.box.padded()
	}
	shade(m, blk.Map)
	return m, nil
}
