package run

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

// survey is where a large map's places are.
type survey struct {
	// box grows with every outline and hexagon drawn around the places;
	// points stays the box of the places alone.
	box, points *Bounds
}

func (q *bigMap) survey(ctx context.Context) (survey, error) {
	p, err := q.engine.Builder.BuildMapSurvey(q.ds, q.blk, q.params, q.filters, q.pr)
	if err != nil {
		return survey{}, err
	}
	sv := survey{box: newBounds(), points: newBounds()}
	_, err = q.each(ctx, p, 0, func(r row) error {
		if r.null("south") {
			return nil
		}
		for _, b := range []*Bounds{sv.box, sv.points} {
			b.add(project(r.num("west"), r.num("north")))
			b.add(project(r.num("east"), r.num("south")))
		}
		// Where flows land, which the map has to hold as well as where they
		// leave from.
		if r.has("tosouth") && !r.null("tosouth") {
			sv.box.add(project(r.num("towest"), r.num("tonorth")))
			sv.box.add(project(r.num("toeast"), r.num("tosouth")))
		}
		return nil
	})
	return sv, err
}

// regions reads the outlines at their own grain, their values exact.
func (q *bigMap) regions(ctx context.Context, m *GeoMap, box *Bounds) error {
	p, err := q.engine.Builder.BuildMapRegions(q.ds, q.blk, q.params, q.filters, q.pr)
	if err != nil {
		return err
	}
	// The small path's reader, over a query shaped like the start of its own:
	// a label, an outline and a value.
	r := &mapReader{
		blk: q.blk, box: box, points: newBounds(), fold: foldOf(q.blk, q.ds), regions: map[string]int{},
		at: map[definition.MapColumn]int{definition.RegionCol: 0, definition.GeometryCol: 1, definition.ValueCol: 2},
	}
	cut, err := q.each(ctx, p, query.ChartLimit, func(row row) error {
		cells := []any{row.vals[0], row.vals[1], row.vals[2]}
		value, _ := number(cells[2])
		return r.shape(cells, m, r.text(cells, definition.RegionCol), value)
	})
	if cut {
		m.Partial = cutAt(group(query.ChartLimit, 0) + " regions")
	}
	return err
}

// hexes reads the hexagons, folded in the database over every place.
func (q *bigMap) hexes(ctx context.Context, m *GeoMap, radius float64, box *Bounds) error {
	p, err := q.engine.Builder.BuildMapHexes(q.ds, q.blk, q.params, q.filters, q.pr, radius)
	if err != nil {
		return err
	}
	type hex struct {
		c hexCell
		n int
		v float64
	}
	var got []hex
	cut, err := q.each(ctx, p, query.MaxCells, func(r row) error {
		got = append(got, hex{c: hexCell{q: int(r.num("q")), r: int(r.num("r"))},
			n: int(r.num("places")), v: r.num("value")})
		return nil
	})
	if err != nil {
		return err
	}
	// In the order the small path lists them, so the two are the same payload
	// for the same places.
	sort.Slice(got, func(i, j int) bool {
		return got[i].c.r < got[j].c.r || (got[i].c.r == got[j].c.r && got[i].c.q < got[j].c.q)
	})
	for _, h := range got {
		m.Hexes = append(m.Hexes, Shape{Label: places(h.n), Path: hexPath(h.c, radius, box),
			Value: h.v, Formatted: compact(h.v)})
		m.Places += h.n
	}
	if cut {
		m.Partial = "These hexagons are too narrow to draw across all of this data — " +
			"widen hexKm or narrow the filters."
	}
	return nil
}

func (q *bigMap) categories(ctx context.Context) ([]string, error) {
	p, err := q.engine.Builder.BuildMapCategories(q.ds, q.blk, q.params, q.filters, q.pr, categoriesKept)
	if err != nil {
		return nil, err
	}
	var names []string
	_, err = q.each(ctx, p, 0, func(r row) error {
		names = append(names, r.text("series"))
		return nil
	})
	return names, err
}

// cells reads the points of one view, gathered into the grid.
func (q *bigMap) cells(ctx context.Context, g query.MapGrid, slot map[string]int) (*Cells, error) {
	p, err := q.engine.Builder.BuildMapCells(q.ds, q.blk, q.params, q.filters, q.pr, g)
	if err != nil {
		return nil, err
	}
	c := &Cells{Size: 1 / float64(g.Cells), Mean: foldOf(q.blk, q.ds) == definition.Unfoldable}
	keyed, sized, labelled := q.blk.Series.Field != "", q.blk.Size.Field != "", q.blk.Labels() != ""
	cut, err := q.each(ctx, p, query.MaxCells, func(r row) error {
		c.X, c.Y = append(c.X, r.num("x")), append(c.Y, r.num("y"))
		c.N, c.V = append(c.N, int(r.num("places"))), append(c.V, r.num("value"))
		if sized {
			c.Z = append(c.Z, r.num("size"))
		}
		if keyed {
			c.S = append(c.S, slotFor(slot, r.text("series")))
		}
		if labelled {
			c.L = append(c.L, r.text("label"))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	c.cut = cut
	c.sort()
	return c, nil
}

// flows reads the routes of one view, busiest first.
func (q *bigMap) flows(ctx context.Context, m *GeoMap, g query.MapGrid, slot map[string]int) error {
	p, err := q.engine.Builder.BuildMapFlows(q.ds, q.blk, q.params, q.filters, q.pr, g)
	if err != nil {
		return err
	}
	keyed, counted := q.blk.Series.Field != "", m.Places == 0
	var values []float64
	cut, err := q.each(ctx, p, query.ChartLimit, func(r row) error {
		v, n := r.num("value"), int(r.num("places"))
		if counted {
			// A map of flows alone is counted by them; one with points too
			// was counted by its cells.
			m.Places += n
		}
		a := Arc{Label: flowLabel(n, r.text("label")),
			X1: r.num("x"), Y1: r.num("y"), X2: r.num("x2"), Y2: r.num("y2"),
			Value: v, Formatted: compact(v)}
		if keyed {
			a.Slot = slotFor(slot, r.text("series"))
		}
		m.Arcs = append(m.Arcs, a)
		values = append(values, v)
		return nil
	})
	lo, hi := span(values)
	for i := range m.Arcs {
		m.Arcs[i].Weight = weight(m.Arcs[i].Value, lo, hi)
	}
	if cut {
		m.Partial = cutAt(group(query.ChartLimit, 0) + " routes, the busiest")
	}
	return err
}

// flowLabel names a route: the one flow it is, or how many it gathers.
func flowLabel(n int, label string) string {
	if n == 1 && label != "" {
		return label
	}
	if n == 1 {
		return "1 flow"
	}
	return group(float64(n), 0) + " flows"
}

// slotFor is a category's swatch, or the last one for a category the map did
// not name when it opened.
func slotFor(slot map[string]int, name string) int {
	if s, ok := slot[name]; ok {
		return s
	}
	return PlotSlots - 1
}

/*
each runs a plan and hands every row to fn by column name, stopping one row past
limit — the row that says there were more — when limit is set.

By name rather than by position because these statements are assembled from
what a block has, and a reader counting columns would have to assemble them the
same way a second time. Lower-cased, because Postgres folds an alias and MySQL
does not.
*/
func (q *bigMap) each(ctx context.Context, p query.Plan, limit int, fn func(row) error) (bool, error) {
	rows, err := q.engine.Executor.Execute(ctx, p)
	if err != nil {
		return false, fmt.Errorf("%w: block %q: %v", ErrExecute, q.blk.Heading(), err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return false, err
	}
	r := row{at: make(map[string]int, len(cols)), vals: make([]any, len(cols))}
	into := make([]any, len(cols))
	for i, c := range cols {
		r.at[strings.ToLower(c)] = i
		into[i] = &r.vals[i]
	}
	for n := 0; rows.Next(); n++ {
		if limit > 0 && n == limit {
			return true, nil
		}
		if err := rows.Scan(into...); err != nil {
			return false, err
		}
		if err := fn(r); err != nil {
			return false, err
		}
	}
	return false, rows.Err()
}

// row is one result row, read by column name.
type row struct {
	at   map[string]int
	vals []any
}

func (r row) has(name string) bool { _, ok := r.at[name]; return ok }

// raw is a column as the driver returned it, for a value only its reader can
// make sense of — an H3 cell, as text or as a number.
func (r row) raw(name string) any {
	if i, ok := r.at[name]; ok {
		return r.vals[i]
	}
	return nil
}

func (r row) null(name string) bool {
	i, ok := r.at[name]
	return !ok || r.vals[i] == nil
}

// num is a column as a number, and zero for anything that is not a finite one:
// a NaN in a payload is JSON no browser parses.
func (r row) num(name string) float64 {
	i, ok := r.at[name]
	if !ok {
		return 0
	}
	v, _ := number(r.vals[i])
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func (r row) text(name string) string {
	if r.null(name) {
		return ""
	}
	return cell(r.vals[r.at[name]])
}
