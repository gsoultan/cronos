package query

import (
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
A map with more places than a browser can hold.

mapSQL reads a map at the grain of a place and caps it at ChartLimit, which is
the right answer for the maps that fit and a wrong one for the rest: a map cut
at five thousand of a million deliveries shows the first five thousand in
whatever order the GROUP BY returned them. So a map that does not fit is asked
different questions — where its places are, and how many of them fall in each
cell of a grid — and every one of them is answered by the database, over every
row, without a row per place crossing the wire.

The statements below all read the same table: pointsSQL, the map's own query
without its cap. What they add is arithmetic over it — Web Mercator in SQL, so a
cell is a square on the screen, and a hexagon's cube rounding, so a hexagon
holds the same places it would have held in Go.
*/

// mapBase compiles the dataset under a block — its parameters, filters and row
// scope — and the block's places over it.
func (b Builder) mapBase(ds definition.Dataset, blk definition.Block, in map[string]any,
	f Filters, pr principal.Principal, box *GeoBox) (string, []any, error) {

	if blk.Map == nil {
		return "", nil, fmt.Errorf("%w: block %q is not a map", ErrBadTemplate, blk.Heading())
	}
	base, _, err := b.BuildWith(ds, in, f, pr)
	if err != nil {
		return "", nil, err
	}
	return b.pointsSQL(ds, blk, base.sql, box, base.args)
}

/*
pointsSQL is a map's rows at the grain of a place, uncapped: a row per label,
coordinate, destination and category, with the block's measures folded over it.

No geometry. A region's outline is read once per region by BuildMapRegions,
and carrying it on every place under it would be the largest column in the
statement, repeated a million times.

The box narrows it to a view, bound as arguments after the dataset's own —
four numbers from a browser, which is exactly what must never be written into
the text of a statement.
*/
func (b Builder) pointsSQL(ds definition.Dataset, blk definition.Block, inner string,
	box *GeoBox, args []any) (string, []any, error) {

	var sel, group []string
	for _, c := range blk.MapColumns() {
		if c == definition.GeometryCol {
			continue
		}
		expr, grouped, err := b.mapColumn(ds, blk, c)
		if err != nil {
			return "", nil, err
		}
		sel = append(sel, expr+" AS "+string(c))
		if grouped {
			group = append(group, expr)
		}
	}
	conds, args, err := b.pointConditions(ds, blk, box, args)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("SELECT %s\nFROM (\n%s\n) AS %s%s\nGROUP BY %s",
		strings.Join(sel, ", "), inner, blockAlias, whereAll(conds),
		strings.Join(group, ", ")), args, nil
}

// pointConditions is the block's own filter and the view's box. A flow is in
// view when either end is, so a route arriving from off the screen still
// arrives.
func (b Builder) pointConditions(ds definition.Dataset, blk definition.Block, box *GeoBox,
	args []any) ([]string, []any, error) {

	var conds []string
	if strings.TrimSpace(blk.Filter) != "" {
		conds = append(conds, "("+blk.Filter+")")
	}
	if box == nil {
		return conds, args, nil
	}
	if !box.Valid() {
		return nil, nil, fmt.Errorf("%w: a view that is not on the globe", ErrBadTemplate)
	}
	// A copy: the dataset's arguments belong to a plan somebody else holds.
	args = append([]any(nil), args...)
	m := blk.Map
	near := func(latField, lonField string) (string, error) {
		lat, err := column(ds, latField)
		if err != nil {
			return "", err
		}
		lon, err := column(ds, lonField)
		if err != nil {
			return "", err
		}
		n := len(args)
		args = append(args, box.South, box.North, box.West, box.East)
		return fmt.Sprintf("(%s BETWEEN %s AND %s AND %s BETWEEN %s AND %s)",
			lat, b.dialect.At(n+1), b.dialect.At(n+2),
			lon, b.dialect.At(n+3), b.dialect.At(n+4)), nil
	}
	from, err := near(m.Lat, m.Lon)
	if err != nil {
		return nil, nil, err
	}
	if !m.Draws(definition.FlowLayer) {
		return append(conds, from), args, nil
	}
	to, err := near(m.ToLat, m.ToLon)
	if err != nil {
		return nil, nil, err
	}
	return append(conds, "("+from+" OR "+to+")"), args, nil
}

// whereAll joins conditions into a WHERE clause, or nothing when there are
// none.
func whereAll(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return "\nWHERE " + strings.Join(conds, " AND ")
}
