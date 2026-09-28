package query

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
BuildMapSurvey asks where a map's places are: the box around every origin and
every destination.

Over the rows rather than the places. A box is the same box either way, and
grouping a million rows into places only to take four extremes of them was most
of what opening a large map cost — on SQLite, four seconds of six. How many
places there are is counted by the cells, which group them anyway.
*/
func (b Builder) BuildMapSurvey(ds definition.Dataset, blk definition.Block,
	in map[string]any, f Filters, pr principal.Principal) (Plan, error) {

	from, args, err := b.mapRows(ds, blk, in, f, pr)
	if err != nil {
		return Plan{}, err
	}
	lat, lon, err := coordinates(ds, blk.Map.Lat, blk.Map.Lon)
	if err != nil {
		return Plan{}, err
	}
	sel := fmt.Sprintf("MIN(%[1]s) AS south, MAX(%[1]s) AS north, MIN(%[2]s) AS west, "+
		"MAX(%[2]s) AS east", lat, lon)
	if blk.Map.Draws(definition.FlowLayer) {
		toLat, toLon, err := coordinates(ds, blk.Map.ToLat, blk.Map.ToLon)
		if err != nil {
			return Plan{}, err
		}
		// Only destinations that are somewhere: a flow to nowhere is not
		// drawn, and its NULL would otherwise be the one MIN ignores and a
		// garbage value the one it does not.
		ok := fmt.Sprintf("%s BETWEEN -90 AND 90 AND %s BETWEEN -180 AND 180", toLat, toLon)
		sel += fmt.Sprintf(", MIN(CASE WHEN %[1]s THEN %[2]s END) AS toSouth, "+
			"MAX(CASE WHEN %[1]s THEN %[2]s END) AS toNorth, "+
			"MIN(CASE WHEN %[1]s THEN %[3]s END) AS toWest, "+
			"MAX(CASE WHEN %[1]s THEN %[3]s END) AS toEast", ok, toLat, toLon)
	}
	return Plan{sql: "SELECT " + sel + from, args: args}, nil
}

// mapRows is the dataset under a block as rows — filtered as the block filters
// it, and only the rows that are somewhere — as a FROM and WHERE clause.
func (b Builder) mapRows(ds definition.Dataset, blk definition.Block, in map[string]any,
	f Filters, pr principal.Principal) (string, []any, error) {

	if blk.Map == nil {
		return "", nil, fmt.Errorf("%w: block %q is not a map", ErrBadTemplate, blk.Heading())
	}
	base, _, err := b.BuildWith(ds, in, f, pr)
	if err != nil {
		return "", nil, err
	}
	lat, lon, err := coordinates(ds, blk.Map.Lat, blk.Map.Lon)
	if err != nil {
		return "", nil, err
	}
	conds := []string{fmt.Sprintf("%s BETWEEN -90 AND 90 AND %s BETWEEN -180 AND 180", lat, lon)}
	if strings.TrimSpace(blk.Filter) != "" {
		conds = append(conds, "("+blk.Filter+")")
	}
	return fmt.Sprintf("\nFROM (\n%s\n) AS %s%s", base.sql, blockAlias, whereAll(conds)),
		base.args, nil
}

// coordinates are two fields of the dataset, checked.
func coordinates(ds definition.Dataset, latField, lonField string) (string, string, error) {
	lat, err := column(ds, latField)
	if err != nil {
		return "", "", err
	}
	lon, err := column(ds, lonField)
	return lat, lon, err
}

/*
BuildMapPlaces is a map's rows at the grain of a place — or of a cell, for an
h3 layer drawing indexed cells — uncapped, for the one fold a warehouse cannot
do in SQL: putting a place in an H3 cell. The reader streams it and keeps the
cells, never the rows, and stops at a cap of its own.
*/
func (b Builder) BuildMapPlaces(ds definition.Dataset, blk definition.Block, in map[string]any,
	f Filters, pr principal.Principal) (Plan, error) {

	sql, args, err := b.mapBase(ds, blk, in, f, pr, nil)
	if err != nil {
		return Plan{}, err
	}
	return Plan{sql: sql, args: args}, nil
}

/*
BuildMapCells gathers a map's places into the cells of a grid.

A row per cell and category: how many places, their measure folded — its own
aggregate applied again, or averaged where it is an average — the middle of
them, and a place's own label when the cell holds one place only. At the depth
a reader zooms to that is most of them, so a cell is a place and the map needs
no second question to show one.
*/
func (b Builder) BuildMapCells(ds definition.Dataset, blk definition.Block, in map[string]any,
	f Filters, pr principal.Principal, g MapGrid) (Plan, error) {

	n, err := gridCells(g.Cells)
	if err != nil {
		return Plan{}, err
	}
	fold, err := foldSQL(ds, blk)
	if err != nil {
		return Plan{}, err
	}
	pts, args, err := b.mapBase(ds, blk, in, f, pr, g.Box)
	if err != nil {
		return Plan{}, err
	}
	cols := blk.MapColumns()
	gx, gy := "FLOOR(c.x * "+n+")", "FLOOR(c.y * "+n+")"
	sel := []string{gx + " AS gx", gy + " AS gy", "COUNT(*) AS places",
		fold + "(c.value) AS value", "AVG(c.x) AS x", "AVG(c.y) AS y"}
	group := []string{gx, gy}
	sel, group = b.cellExtras(blk, cols, fold, sel, group)

	top, tail := b.dialect.Limit(MaxCells + 1)
	return Plan{sql: fmt.Sprintf("SELECT %s%s\nFROM (\n%s\n) AS c\nGROUP BY %s%s",
		top, strings.Join(sel, ", "), b.projected(pts, cols, false),
		strings.Join(group, ", "), tail), args: args}, nil
}

// cellExtras adds what a cell carries when the block has it: a category, a
// size, and a lone place's label.
func (b Builder) cellExtras(blk definition.Block, cols []definition.MapColumn, fold string,
	sel, group []string) ([]string, []string) {

	if has(cols, definition.SeriesCol) {
		sel = append(sel, "c.series AS series")
		group = append(group, "c.series")
	}
	if has(cols, definition.SizeCol) {
		sel = append(sel, fold+"(c.size) AS size")
	}
	if has(cols, definition.RegionCol) {
		sel = append(sel, "CASE WHEN COUNT(*) = 1 THEN MIN(c.region) END AS label")
	}
	return sel, group
}

/*
BuildMapFlows gathers flows into routes between cells: a row per origin cell,
destination cell and category, largest first, so the cap keeps the routes that
carry the most and a reader zooming in splits them into the flows they are.
*/
func (b Builder) BuildMapFlows(ds definition.Dataset, blk definition.Block, in map[string]any,
	f Filters, pr principal.Principal, g MapGrid) (Plan, error) {

	n, err := gridCells(g.Cells)
	if err != nil {
		return Plan{}, err
	}
	fold, err := foldSQL(ds, blk)
	if err != nil {
		return Plan{}, err
	}
	pts, args, err := b.mapBase(ds, blk, in, f, pr, g.Box)
	if err != nil {
		return Plan{}, err
	}
	cols := blk.MapColumns()
	group := []string{"FLOOR(c.x * " + n + ")", "FLOOR(c.y * " + n + ")",
		"FLOOR(c.x2 * " + n + ")", "FLOOR(c.y2 * " + n + ")"}
	sel := []string{"COUNT(*) AS places", fold + "(c.value) AS value",
		"AVG(c.x) AS x", "AVG(c.y) AS y", "AVG(c.x2) AS x2", "AVG(c.y2) AS y2"}
	sel, group = b.cellExtras(blk, cols, fold, sel, group)

	top, tail := b.dialect.Limit(ChartLimit + 1)
	return Plan{sql: fmt.Sprintf("SELECT %s%s\nFROM (\n%s\n) AS c\nGROUP BY %s\nORDER BY 2 DESC%s",
		top, strings.Join(sel, ", "), b.projected(pts, cols, true),
		strings.Join(group, ", "), tail), args: args}, nil
}

/*
BuildMapHexes folds a map's places into hexagons of the given radius, in world
units: the hexbin layer, over every row rather than the first five thousand.

The cube rounding run.cellOf does, in SQL — axial coordinates, rounded through
cube coordinates so a place on an edge lands in exactly one hexagon, and in the
same one Go would have put it in. FLOOR(v + 0.5) stands for Go's round-half-
away-from-zero, which disagrees only on an exact tie below zero; ROUND would
disagree more, because Postgres rounds a double to even.
*/
func (b Builder) BuildMapHexes(ds definition.Dataset, blk definition.Block, in map[string]any,
	f Filters, pr principal.Principal, radius float64) (Plan, error) {

	if !(radius > 0) || radius > 1 {
		return Plan{}, fmt.Errorf("%w: a hexagon %g of the world across", ErrBadTemplate, radius)
	}
	fold, err := foldSQL(ds, blk)
	if err != nil {
		return Plan{}, err
	}
	pts, args, err := b.mapBase(ds, blk, in, f, pr, nil)
	if err != nil {
		return Plan{}, err
	}
	r := literal(radius)
	frac := fmt.Sprintf("SELECT (5.773502691896258E-1 * c.x - c.y / 3) / %[1]s AS qf, "+
		"(c.y * 2 / 3) / %[1]s AS rf, c.value AS value\nFROM (\n%[2]s\n) AS c", r,
		b.projected(pts, blk.MapColumns(), false))
	near := "SELECT FLOOR(h.qf + 0.5) AS cx, FLOOR(-h.qf - h.rf + 0.5) AS cy, " +
		"FLOOR(h.rf + 0.5) AS cz, h.qf AS qf, h.rf AS rf, h.value AS value\nFROM (\n" + frac + "\n) AS h"
	split := "k.dx > k.dy AND k.dx > k.dz"
	cells := fmt.Sprintf("SELECT CASE WHEN %[1]s THEN -k.cy - k.cz ELSE k.cx END AS q, "+
		"CASE WHEN %[1]s THEN k.cz WHEN k.dy <= k.dz THEN -k.cx - k.cy ELSE k.cz END AS r, "+
		"k.value AS value\nFROM (\nSELECT d.cx AS cx, d.cy AS cy, d.cz AS cz, "+
		"ABS(d.cx - d.qf) AS dx, ABS(d.cy + d.qf + d.rf) AS dy, ABS(d.cz - d.rf) AS dz, "+
		"d.value AS value\nFROM (\n%[2]s\n) AS d\n) AS k", split, near)

	top, tail := b.dialect.Limit(MaxCells + 1)
	return Plan{sql: fmt.Sprintf("SELECT %sw.q AS q, w.r AS r, COUNT(*) AS places, "+
		"%s(w.value) AS value\nFROM (\n%s\n) AS w\nGROUP BY w.q, w.r%s",
		top, fold, cells, tail), args: args}, nil
}

/*
BuildMapRegions reads a map's regions at their own grain: a row per region and
outline, its measure over the rows under it.

What Grouped cannot do at a place's grain for a map too large to read whole —
and exactly, for every aggregate: an average of a region's rows is taken over
the rows, so there is no average of averages to refuse.
*/
func (b Builder) BuildMapRegions(ds definition.Dataset, blk definition.Block,
	in map[string]any, f Filters, pr principal.Principal) (Plan, error) {

	base, _, err := b.BuildWith(ds, in, f, pr)
	if err != nil {
		return Plan{}, err
	}
	label, err := column(ds, blk.Labels())
	if err != nil {
		return Plan{}, err
	}
	geometry, err := column(ds, blk.Map.Geometry)
	if err != nil {
		return Plan{}, err
	}
	value, err := b.measure(ds, blk.Y)
	if err != nil {
		return Plan{}, err
	}
	top, tail := b.dialect.Limit(ChartLimit + 1)
	return Plan{sql: fmt.Sprintf("SELECT %s%s AS region, %s AS geometry, %s AS value\n"+
		"FROM (\n%s\n) AS %s%s\nGROUP BY %s, %s\nORDER BY 1%s",
		top, label, geometry, value, base.sql, blockAlias, where(blk.Filter),
		label, geometry, tail), args: base.args}, nil
}

/*
BuildMapCategories is a map's categories, the most common first.

A large map is drawn a view at a time, and each view is its own query — so the
colour each category gets cannot come from the order a view's rows arrive in,
or a carrier would change colour as the reader pans. It comes from here, once.
Counted over rows rather than places, which orders them the same way in any
map anybody would draw, for a third of the cost.
*/
func (b Builder) BuildMapCategories(ds definition.Dataset, blk definition.Block,
	in map[string]any, f Filters, pr principal.Principal, limit int) (Plan, error) {

	from, args, err := b.mapRows(ds, blk, in, f, pr)
	if err != nil {
		return Plan{}, err
	}
	series, err := column(ds, blk.Series.Field)
	if err != nil {
		return Plan{}, err
	}
	top, tail := b.dialect.Limit(limit)
	return Plan{sql: fmt.Sprintf("SELECT %s%s AS series, COUNT(*) AS n%s\nGROUP BY %s\nORDER BY 2 DESC, 1%s",
		top, series, from, series, tail), args: args}, nil
}

/*
projected is the places with their coordinates in Web Mercator world units —
x and y, and x2 and y2 for a flow's destination — the space every tile and
every mark on a map is drawn in.

The same numbers run.project computes, for rows that never leave the database.
Written through the sine, since ln(tan(π/4 + φ/2)) is ½·ln((1 + sin φ)/(1 − sin
φ)), so the logarithm is the only function whose name differs between
databases. Clamped to Web Mercator's limit first as project clamps, so a place
in the Arctic lands on the top edge rather than dividing by nothing at the pole.

The constants are written with an exponent, because that makes them floating
point in SQL Server, MySQL and DuckDB — where RADIANS of an integer column is an
integer — and every database here parses them.
*/
func (b Builder) projected(pts string, cols []definition.MapColumn, flows bool) string {
	sel := []string{mercatorX("p.lon") + " AS x", b.mercatorY("p.lat") + " AS y", "p.value AS value"}
	where := located("p")
	if flows {
		sel = append(sel, mercatorX("p.toLon")+" AS x2", b.mercatorY("p.toLat")+" AS y2")
		where += " AND p.toLat BETWEEN -90 AND 90 AND p.toLon BETWEEN -180 AND 180"
	}
	for _, c := range []definition.MapColumn{definition.RegionCol, definition.SeriesCol, definition.SizeCol} {
		if has(cols, c) {
			sel = append(sel, "p."+string(c)+" AS "+string(c))
		}
	}
	return fmt.Sprintf("SELECT %s\nFROM (\n%s\n) AS p\nWHERE %s", strings.Join(sel, ", "), pts, where)
}

// mercatorX is a longitude's x: 180°W is 0 and 180°E is 1.
func mercatorX(lon string) string {
	return "((" + lon + " + 1.8E2) * 2.777777777777778E-3)"
}

// mercatorY is a latitude's y: the northern cut is 0 and the southern 1.
func (b Builder) mercatorY(lat string) string {
	clamped := fmt.Sprintf("CASE WHEN %[1]s > 8.505112878E1 THEN 8.505112878E1 "+
		"WHEN %[1]s < -8.505112878E1 THEN -8.505112878E1 ELSE %[1]s END", lat)
	sin := "SIN((" + clamped + ") * 1.7453292519943295E-2)"
	return "(5E-1 - " + b.dialect.Ln("(1 + "+sin+") / (1 - "+sin+")") + " * 7.957747154594767E-2)"
}

// located keeps the places that are somewhere: a NULL coordinate fails BETWEEN,
// and one off the globe is a typo rather than a place.
func located(alias string) string {
	return alias + ".lat BETWEEN -90 AND 90 AND " + alias + ".lon BETWEEN -180 AND 180"
}

// foldSQL is the aggregate that folds a measure a second time — SUM for a sum
// or a count, MIN, MAX — and AVG for the one that cannot be, whose cells say
// they are an average of places rather than of rows.
func foldSQL(ds definition.Dataset, blk definition.Block) (string, error) {
	name := blk.Y.Aggregate
	if name == "" {
		fld, ok := ds.Field(blk.Y.Field)
		if !ok {
			return "", fmt.Errorf("%w: %q is not a field of dataset %q",
				ErrBadTemplate, blk.Y.Field, ds.Name)
		}
		name = fld.Aggregate
	}
	switch definition.Foldable(name) {
	case definition.FoldSum:
		return "SUM", nil
	case definition.FoldMin:
		return "MIN", nil
	case definition.FoldMax:
		return "MAX", nil
	}
	return "AVG", nil
}

// gridCells is a grid's width as SQL: a power of two, from a caller.
func gridCells(n int) (string, error) {
	if n < 1 || n > 1<<30 || n&(n-1) != 0 {
		return "", fmt.Errorf("%w: a grid %d cells across", ErrBadTemplate, n)
	}
	return strconv.Itoa(n), nil
}

// literal writes a number the server computed as SQL: with an exponent, so it
// is floating point wherever that is a choice.
func literal(v float64) string { return strconv.FormatFloat(v, 'E', -1, 64) }

func has(cols []definition.MapColumn, c definition.MapColumn) bool {
	for _, have := range cols {
		if have == c {
			return true
		}
	}
	return false
}
