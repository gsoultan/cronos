package query

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// mapSQL compiles a map block.
//
// The grain is whatever the finest requested layer needs: a point layer means
// a row per coordinate, a polygon layer alone means a row per region. Asking
// for both runs at the point grain and folds the regions from it — see
// definition.Block.Grouped for why that is one query rather than two, and
// foldable for the aggregate it refuses.
//
// Columns come out in definition.Block.MapColumns order, which is the contract
// the reader scans them back by.
func (b Builder) mapSQL(ds definition.Dataset, blk definition.Block, inner string) (string, error) {
	if err := foldable(ds, blk); err != nil {
		return "", err
	}
	sel := make([]string, 0, 9)
	group := make([]string, 0, 6)

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
	return b.wrap(sel, inner, blk, group, b.order(ds, blk, len(group)))
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
	if !blk.Grouped() {
		return nil
	}
	name := blk.Y.Aggregate
	if name == "" {
		f, ok := ds.Field(blk.Y.Field)
		if !ok {
			return fmt.Errorf("%w: %q is not a field of dataset %q",
				ErrBadTemplate, blk.Y.Field, ds.Name)
		}
		name = f.Aggregate
	}
	if definition.Foldable(name) == definition.Unfoldable {
		return fmt.Errorf("%w: map block shades polygons and draws points, so it runs "+
			"per point and adds each region up from those — which %q cannot survive. "+
			"Use sum, count, min or max, or split the layers across two blocks",
			ErrBadTemplate, name)
	}
	return nil
}
