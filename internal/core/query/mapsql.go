package query

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// mapSQL compiles a map block.
//
// The grain is whatever the finest requested layer needs: a point layer means
// a row per coordinate, a polygon or line layer alone means a row per shape.
// Asking for both runs at the point grain and folds the shapes from it — see
// definition.Block.Grouped for why that is one query rather than two, and
// foldable for the aggregate it refuses. A hexbin layer folds too, but in the
// reader: a hexagon is a place on the map, not a value any column holds.
//
// Columns come out in definition.Block.MapColumns order, which is the contract
// the reader scans them back by.
func (b Builder) mapSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	if err := foldable(ds, blk); err != nil {
		return "", err
	}
	sel := make([]string, 0, 10)
	group := make([]string, 0, 7)

	for _, c := range blk.MapColumns() {
		expr, grouped, err := b.mapColumn(ds, blk, c)
		if err != nil {
			return "", err
		}
		sel = append(sel, expr+" AS "+string(c))
		if grouped {
			group = append(group, expr)
		}
	}
	// One row past the cap. A map folds its rows into totals — a region's,
	// a hexagon's — so a map cut at the cap is not a smaller picture of the
	// data but a wrong one, and the reader can only say so if it can tell a
	// map that fit from one that did not.
	return b.capped(sel, inner, blk, group, b.order(ds, blk, len(group)), ChartLimit+1)
}

// mapColumn renders one column and says whether it is grouped by rather than
// folded. Every dimension is; every measure is not.
func (b Builder) mapColumn(ds definition.Dataset, blk definition.Block,
	c definition.MapColumn) (string, bool, error) {

	m := blk.Map
	switch c {
	case definition.RegionCol:
		col, err := column(ds, blk.Labels())
		return col, true, err
	case definition.GeometryCol:
		col, err := column(ds, m.Geometry)
		return col, true, err
	case definition.LatCol:
		col, err := column(ds, m.Lat)
		return col, true, err
	case definition.LonCol:
		col, err := column(ds, m.Lon)
		return col, true, err
	case definition.ToLatCol:
		col, err := column(ds, m.ToLat)
		return col, true, err
	case definition.ToLonCol:
		col, err := column(ds, m.ToLon)
		return col, true, err
	case definition.SeriesCol:
		col, err := column(ds, blk.Series.Field)
		return col, true, err
	case definition.H3Col:
		col, err := column(ds, m.H3)
		return col, true, err
	case definition.TimeCol:
		col, err := column(ds, m.Time.Field)
		if err != nil {
			return "", false, err
		}
		expr, err := b.dialect.Bucket(m.Time.Grain, col)
		return expr, true, err
	case definition.ValueCol:
		expr, err := b.measure(ds, blk.Y)
		return expr, false, err
	case definition.SizeCol:
		expr, err := b.measure(ds, blk.Size)
		return expr, false, err
	}
	return "", false, fmt.Errorf("%w: %q is not a map column", ErrBadTemplate, c)
}

// foldable refuses a map that would average averages.
//
// definition.validateFold runs the same check on what the author wrote; this
// runs it on what the aggregate resolves to, which is the dataset's default
// when the block named none — the case that reaches a reader as a plausible
// wrong number rather than as an error.
func foldable(ds definition.Dataset, blk definition.Block) error {
	if !blk.Folds() {
		return nil
	}
	for _, m := range blk.FoldedMeasures() {
		name := m.Aggregate
		if name == "" {
			f, ok := ds.Field(m.Field)
			if !ok {
				return fmt.Errorf("%w: %q is not a field of dataset %q",
					ErrBadTemplate, m.Field, ds.Name)
			}
			name = f.Aggregate
		}
		if definition.Foldable(name) == definition.Unfoldable {
			return fmt.Errorf("%w: map block %s — which %q cannot survive. "+
				"Use sum, count, min or max, or split the layers across two blocks",
				ErrBadTemplate, blk.FoldReason(), name)
		}
	}
	return nil
}
