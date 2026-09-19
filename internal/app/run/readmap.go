package run

import (
	"github.com/gsoultan/cronos/internal/core/definition"
)

// readMap reads a map block's rows into the layers a viewer draws.
//
// The rows arrive in definition.Block.MapColumns order, which is the one place
// the shape is decided — the compiler wrote the SELECT from the same list.
func readMap(blk definition.Block, ds definition.Dataset, rows Rows) (*GeoMap, error) {
	cols := blk.MapColumns()
	at := make(map[definition.MapColumn]int, len(cols))
	for i, c := range cols {
		at[c] = i
	}

	m := blk.Map
	out := &GeoMap{
		Layers:  layerNames(m),
		Shapes:  []Shape{},
		Markers: []Marker{},
		Arcs:    []Arc{},
		Legend:  []Legend{},
	}
	if m.Basemap != nil {
		out.Tiles = &Tiles{
			URL:         m.Basemap.URL,
			Attribution: m.Basemap.Attribution,
			MaxZoom:     m.Basemap.Zoom(),
		}
	}

	r := &mapReader{blk: blk, at: at, box: newBounds(), fold: foldOf(blk, ds), regions: map[string]int{}}
	if err := r.scan(rows, len(cols), out); err != nil {
		return nil, err
	}
	r.finish(out)
	return out, nil
}

// mapReader carries the state accumulating a map takes: the box every layer
// grows, and the region totals a polygon layer folds when the query ran at
// point grain.
type mapReader struct {
	blk  definition.Block
	at   map[definition.MapColumn]int
	box  *Bounds
	fold definition.Fold
	// regions indexes GeoMap.Shapes by label, so a second row for a region
	// folds into the shape already made rather than drawing it twice.
	regions map[string]int
	weights []float64
	arcAt   []int
}

func (r *mapReader) scan(rows Rows, width int, out *GeoMap) error {
	for rows.Next() {
		cells := make([]any, width)
		into := make([]any, width)
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := rows.Scan(into...); err != nil {
			return err
		}
		if err := r.row(cells, out); err != nil {
			return err
		}
	}
	return rows.Err()
}

// row folds one result row into whichever layers read it.
func (r *mapReader) row(cells []any, out *GeoMap) error {
	label := r.text(cells, definition.RegionCol)
	value, _ := number(r.get(cells, definition.ValueCol))

	if _, ok := r.at[definition.GeometryCol]; ok {
		if err := r.shape(cells, out, label, value); err != nil {
			return err
		}
	}
	if _, ok := r.at[definition.LatCol]; ok {
		r.point(cells, out, label, value)
	}
	return nil
}

// shape adds or folds one polygon.
func (r *mapReader) shape(cells []any, out *GeoMap, label string, value float64) error {
	if i, seen := r.regions[label]; seen {
		// The same region again, because the query ran at point grain. Its
		// geometry is already drawn; only the value has to catch up.
		out.Shapes[i].Value = refold(r.fold, out.Shapes[i].Value, value)
		return nil
	}
	raw := r.text(cells, definition.GeometryCol)
	if raw == "" {
		// A region with no geometry is a row the join did not match. Skipping
		// it draws the map that does match rather than failing the block.
		return nil
	}
	path, err := geoPath(raw, r.blk.Map.Tolerance(), r.box)
	if err != nil {
		return err
	}
	r.regions[label] = len(out.Shapes)
	out.Shapes = append(out.Shapes, Shape{Label: label, Path: path, Value: value})
	return nil
}

// point adds one marker, and the arc that leaves it when flows are drawn.
func (r *mapReader) point(cells []any, out *GeoMap, label string, value float64) {
	lon, okLon := number(r.get(cells, definition.LonCol))
	lat, okLat := number(r.get(cells, definition.LatCol))
	if !okLon || !okLat || !finite(lon, lat) {
		// A row with no coordinate is not a row at (0, 0) — that is the Gulf
		// of Guinea, and a thousand unaddressed records clustered there is the
		// most confident wrong answer a map can give.
		return
	}
	x, y := project(lon, lat)
	r.box.add(x, y)

	m := Marker{Label: label, X: x, Y: y, Value: value, Formatted: compact(value)}
	if i, ok := r.at[definition.SizeCol]; ok {
		s, _ := number(cells[i])
		m.Size = compact(s)
	}
	out.Markers = append(out.Markers, m)
	r.weights = append(r.weights, value)

	if _, ok := r.at[definition.ToLatCol]; ok {
		r.arc(cells, out, label, value, x, y)
	}
}

func (r *mapReader) arc(cells []any, out *GeoMap, label string, value, x, y float64) {
	lon, okLon := number(r.get(cells, definition.ToLonCol))
	lat, okLat := number(r.get(cells, definition.ToLatCol))
	if !okLon || !okLat || !finite(lon, lat) {
		return
	}
	x2, y2 := project(lon, lat)
	r.box.add(x2, y2)
	r.arcAt = append(r.arcAt, len(out.Arcs))
	out.Arcs = append(out.Arcs, Arc{
		Label: label, X1: x, Y1: y, X2: x2, Y2: y2,
		Value: value, Formatted: compact(value),
	})
}

// finish computes everything that needed every row: the box, the ramp the
// polygons are shaded from, and the weights a bubble and a flow are sized by.
func (r *mapReader) finish(out *GeoMap) {
	out.Bounds = r.box.padded()

	values := make([]float64, len(out.Shapes))
	for i, s := range out.Shapes {
		values[i] = s.Value
	}
	breaks := steps(values)
	for i := range out.Shapes {
		out.Shapes[i].Step = stepOf(out.Shapes[i].Value, breaks)
		out.Shapes[i].Formatted = compact(out.Shapes[i].Value)
	}
	out.Legend = legendOf(values, breaks)

	lo, hi := span(r.weights)
	for i := range out.Markers {
		out.Markers[i].Weight = weight(out.Markers[i].Value, lo, hi)
	}
	for _, i := range r.arcAt {
		out.Arcs[i].Weight = weight(out.Arcs[i].Value, lo, hi)
	}
}

func (r *mapReader) get(cells []any, c definition.MapColumn) any {
	i, ok := r.at[c]
	if !ok {
		return nil
	}
	return cells[i]
}

func (r *mapReader) text(cells []any, c definition.MapColumn) string {
	if v := r.get(cells, c); v != nil {
		return cell(v)
	}
	return ""
}

// refold applies the measure's own aggregate a second time — see
// definition.Block.Grouped.
func refold(f definition.Fold, a, b float64) float64 {
	switch f {
	case definition.FoldMin:
		return min(a, b)
	case definition.FoldMax:
		return max(a, b)
	}
	return a + b
}

// foldOf resolves the aggregate, preferring the block's over the field's.
func foldOf(blk definition.Block, ds definition.Dataset) definition.Fold {
	name := blk.Y.Aggregate
	if name == "" {
		if f, ok := ds.Field(blk.Y.Field); ok {
			name = f.Aggregate
		}
	}
	return definition.Foldable(name)
}

// legendOf labels each band of the ramp with the values it covers.
func legendOf(values []float64, breaks []float64) []Legend {
	if len(values) == 0 {
		return []Legend{}
	}
	lo, hi := span(values)
	out := make([]Legend, 0, len(breaks)+1)
	from := lo
	for i, b := range breaks {
		out = append(out, Legend{Step: i, From: compact(from), To: compact(b)})
		from = b
	}
	return append(out, Legend{Step: len(breaks), From: compact(from), To: compact(hi)})
}

// layerNames is the resolved layer list as the wire carries it.
func layerNames(m *definition.MapSpec) []string {
	resolved := m.Resolved()
	out := make([]string, len(resolved))
	for i, l := range resolved {
		out[i] = string(l)
	}
	return out
}
