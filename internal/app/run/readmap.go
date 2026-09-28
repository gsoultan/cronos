package run

import (
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

// readMap reads a map block's rows into the layers a viewer draws.
//
// The rows arrive in definition.Block.MapColumns order, which is the one place
// the shape is decided — the compiler wrote the SELECT from the same list.
//
// The basemap is not read here. Resolving it can mean a key and a network
// call, which is the Service's business and only for an output a browser
// draws — see Service.tiles.
func readMap(blk definition.Block, ds definition.Dataset, rows Rows) (*GeoMap, error) {
	cols := blk.MapColumns()
	at := make(map[definition.MapColumn]int, len(cols))
	for i, c := range cols {
		at[c] = i
	}

	out := emptyMap(blk)
	r := &mapReader{
		blk: blk, at: at, box: newBounds(), points: newBounds(),
		fold: foldOf(blk.Y, ds), regions: map[string]int{},
	}
	if blk.Map.Draws(definition.H3Layer) {
		r.cells = newH3Cells(blk.Map, r.fold)
	}
	if blk.Map.Time != nil {
		r.timed = newTimeline(blk.Map.Time, foldOf(blk.Size, ds))
	}
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
	blk definition.Block
	at  map[definition.MapColumn]int
	box *Bounds
	// points is the box of the points alone, which is what a hexagon is sized
	// against — the padded box would grow every time it was asked.
	points *Bounds
	fold   definition.Fold
	// regions indexes the shape or line already made for a label, so a second
	// row for it folds into that rather than drawing it twice.
	regions map[string]int
	lined   map[string]int
	weights []float64
	arcAt   []int
	// categories is each marker's series value, in row order, for the slots
	// finish assigns once every row has been seen.
	categories []string
	// cut is whether the query had more rows than a map holds.
	cut bool
	// cells gathers an h3 layer's cells; nil for a map without one.
	cells *h3Cells
	// timed gathers a timed map's periods, and now is the row's; nil and -1
	// for a map that does not play through time.
	timed *timeline
	now   int
}

func (r *mapReader) scan(rows Rows, width int, out *GeoMap) error {
	read := 0
	for rows.Next() {
		if read == query.ChartLimit {
			// The query asked for one row past the cap, and this is it.
			r.cut = true
			break
		}
		read++
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
	r.now = -1
	if r.timed != nil {
		r.now = r.timed.period(r.get(cells, definition.TimeCol))
	}

	if _, ok := r.at[definition.GeometryCol]; ok {
		if err := r.shape(cells, out, label, value); err != nil {
			return err
		}
		if r.timed != nil {
			r.timed.region(label, r.now, value, r.fold)
		}
	}
	if _, ok := r.at[definition.LatCol]; ok {
		r.point(cells, out, label, value)
	}
	if _, ok := r.at[definition.H3Col]; ok {
		r.cells.cell(r.get(cells, definition.H3Col), value)
	}
	return nil
}

// shape adds or folds one polygon or line.
func (r *mapReader) shape(cells []any, out *GeoMap, label string, value float64) error {
	if i, seen := r.regions[label]; seen {
		// The same region again, because the query ran at point grain. Its
		// geometry is already drawn; only the value has to catch up.
		out.Shapes[i].Value = refold(r.fold, out.Shapes[i].Value, value)
		return nil
	}
	if i, seen := r.lined[label]; seen {
		out.Lines[i].Value = refold(r.fold, out.Lines[i].Value, value)
		return nil
	}
	raw := r.text(cells, definition.GeometryCol)
	if raw == "" {
		// A region with no geometry is a row the join did not match. Skipping
		// it draws the map that does match rather than failing the block.
		return nil
	}
	path, kind, err := geoPath(raw, r.blk.Map.Tolerance(), r.box)
	if err != nil {
		return err
	}
	return r.place(out, Shape{Label: label, Path: path, Value: value}, kind)
}

// place files a shape under the layer that draws its kind of geometry.
//
// One field can hold both — districts and the roads between them — and each
// layer draws its own. A kind no requested layer draws is the author pointing
// the block at the wrong column, which is worth a sentence rather than a map
// with nothing on it.
func (r *mapReader) place(out *GeoMap, s Shape, kind geoKind) error {
	m := r.blk.Map
	switch {
	case kind == areaGeometry && m.Draws(definition.PolygonLayer):
		r.regions[s.Label] = len(out.Shapes)
		out.Shapes = append(out.Shapes, s)
	case kind == lineGeometry && m.Draws(definition.LineLayer):
		if r.lined == nil {
			r.lined = map[string]int{}
		}
		r.lined[s.Label] = len(out.Lines)
		out.Lines = append(out.Lines, s)
	case kind == areaGeometry:
		return fmt.Errorf("%w: %q is an area, and this map draws lines from its "+
			"geometry — add a polygon layer to shade it", ErrNotRenderable, s.Label)
	default:
		return fmt.Errorf("%w: %q is a line, and a polygon layer shades areas — "+
			"add a line layer to draw it", ErrNotRenderable, s.Label)
	}
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
	r.points.add(x, y)
	if r.cells != nil && r.cells.binned {
		r.cells.place(lat, lon, value)
	}

	at := r.marker(cells, out, label, value, x, y)
	if _, ok := r.at[definition.ToLatCol]; ok {
		r.arc(cells, out, label, value, at)
	}
}

// marker adds a place's marker, or — on a timed map, where a place arrives
// once for each period — folds a period into the one it already has. Returns
// the marker's index.
func (r *mapReader) marker(cells []any, out *GeoMap, label string, value, x, y float64) int {
	series := ""
	if _, ok := r.at[definition.SeriesCol]; ok {
		series = r.text(cells, definition.SeriesCol)
	}
	size, sized := r.sizeOf(cells)
	if r.timed != nil {
		key := placeKey{label, series, x, y}
		if i, seen := r.timed.places[key]; seen {
			r.again(&out.Markers[i], i, value, size, sized)
			return i
		}
		r.timed.place(key, len(out.Markers), r.now, value, size, sized)
	}
	m := Marker{Label: label, X: onGrid(x), Y: onGrid(y), Value: value, Formatted: compact(value)}
	if sized {
		m.Size = compact(size)
	}
	out.Markers = append(out.Markers, m)
	r.weights = append(r.weights, value)
	if _, ok := r.at[definition.SeriesCol]; ok {
		r.categories = append(r.categories, series)
	}
	return len(out.Markers) - 1
}

// sizeOf is a row's size measure, when the map has one.
func (r *mapReader) sizeOf(cells []any) (float64, bool) {
	i, ok := r.at[definition.SizeCol]
	if !ok {
		return 0, false
	}
	s, _ := number(cells[i])
	return s, true
}

// again folds a timed place's row in another period into its marker: into
// what the map says as it opens, and into that period.
func (r *mapReader) again(m *Marker, i int, value, size float64, sized bool) {
	m.Value = refold(r.fold, m.Value, value)
	m.Formatted, r.weights[i] = compact(m.Value), m.Value
	addPeriod(r.timed.marker[i], r.now, value, r.fold)
	if sized {
		m.Size = compact(r.timed.sized(i, r.now, size))
	}
}

// arc adds the flow leaving a marker, folding a timed map's periods into one.
func (r *mapReader) arc(cells []any, out *GeoMap, label string, value float64, from int) {
	lon, okLon := number(r.get(cells, definition.ToLonCol))
	lat, okLat := number(r.get(cells, definition.ToLatCol))
	if !okLon || !okLat || !finite(lon, lat) {
		return
	}
	x, y := out.Markers[from].X, out.Markers[from].Y
	x2, y2 := project(lon, lat)
	r.box.add(x2, y2)
	if r.timed != nil {
		key := routeKey{label, r.text(cells, definition.SeriesCol), x, y, x2, y2}
		if i, seen := r.timed.routes[key]; seen {
			a := &out.Arcs[i]
			a.Value = refold(r.fold, a.Value, value)
			a.Formatted = compact(a.Value)
			addPeriod(r.timed.arc[i], r.now, value, r.fold)
			return
		}
		r.timed.routes[key] = len(out.Arcs)
		r.timed.arc = append(r.timed.arc, map[int]float64{r.now: value})
	}
	r.arcAt = append(r.arcAt, from)
	out.Arcs = append(out.Arcs, Arc{
		Label: label, X1: x, Y1: y, X2: onGrid(x2), Y2: onGrid(y2),
		Value: value, Formatted: compact(value),
	})
}

// finish computes everything that needed every row: the hexagons, the box,
// the ramp the shaded layers are coloured from, and the weights a bubble and
// a flow are sized by.
func (r *mapReader) finish(out *GeoMap) {
	m := r.blk.Map
	if m.Draws(definition.HexbinLayer) {
		out.Hexes = hexbin(out.Markers, hexRadius(m.HexKm, r.points), r.fold, r.box)
	}
	if r.cells != nil {
		out.Hexes = r.cells.shapes(r.points, r.box)
	}
	out.Bounds = r.box.padded()

	shade(out, r.blk.Map)
	r.paint(out)

	lo, hi := span(r.weights)
	for i := range out.Markers {
		out.Markers[i].Weight = weight(out.Markers[i].Value, lo, hi)
	}
	for i := range out.Arcs {
		out.Arcs[i].Weight = weight(out.Arcs[i].Value, lo, hi)
	}
	if r.timed != nil {
		r.timed.finish(out, m)
	}
	if !markersDrawn(m) {
		// Read for the hexagons or the flows and drawn by nothing. Sending
		// them anyway was up to five thousand points of payload that every
		// viewer parsed and ignored.
		out.Markers = []Marker{}
	}
	if r.cut {
		out.Partial = partial(r.blk)
		out.cut = true
	}
}

// partial is the sentence under a map drawn from the first rows of more.
func partial(blk definition.Block) string {
	if blk.Map != nil && blk.Map.Time != nil {
		return fmt.Sprintf("This map plays the first %s places and periods of more — narrow the "+
			"filters, or play it by a longer period, to see them all.", group(query.ChartLimit, 0))
	}
	first := fmt.Sprintf("This map shows the first %s places of more", group(query.ChartLimit, 0))
	if blk.Folds() {
		return first + ", and its totals count only those — narrow the filters to count them all."
	}
	return first + " — narrow the filters to see the rest."
}

// shade gives every ramped mark its step, over one set of breaks.
//
// One set for regions and lines together: they are the same measure over the
// same rows, so the same colour has to mean the same value on both. Hexagons
// never share a map with either — definition.MapSpec refuses it — because a
// hexagon's value is a fold of points and not a row.
func shade(out *GeoMap, m *definition.MapSpec) {
	marks := make([]*Shape, 0, len(out.Shapes)+len(out.Lines)+len(out.Hexes))
	for _, list := range [][]Shape{out.Shapes, out.Lines, out.Hexes} {
		for i := range list {
			marks = append(marks, &list[i])
		}
	}
	values := make([]float64, len(marks))
	for i, s := range marks {
		values[i] = s.Value
	}
	ramp := shadesFor(m, values)
	for _, s := range marks {
		s.Step = ramp.shade(s.Value)
		s.Formatted = compact(s.Value)
	}
	out.Legend = ramp.legend(values)
	if ramp.diverging {
		out.Ramp = string(definition.DivergingRamp)
	}
}

// paint gives each marker and flow its category's colour, and names them.
//
// Slots by order of first appearance and capped at PlotSlots, the same rule a
// scatter follows and for the same reason: on a map every category sits beside
// every other, and only the first three slots of the palette hold apart
// against all the rest.
func (r *mapReader) paint(out *GeoMap) {
	if len(r.categories) == 0 {
		return
	}
	slot := slots(r.categories, PlotSlots)
	for i := range out.Markers {
		out.Markers[i].Slot = slot[r.categories[i]]
	}
	shown := r.categories
	if !markersDrawn(r.blk.Map) {
		// Only the flows are drawn, and a row with no destination is no flow.
		// A legend naming a category nothing on the map is coloured in is a
		// key to something that is not there.
		shown = make([]string, 0, len(r.arcAt))
	}
	for i, at := range r.arcAt {
		out.Arcs[i].Slot = slot[r.categories[at]]
		if !markersDrawn(r.blk.Map) {
			shown = append(shown, r.categories[at])
		}
	}
	out.Keys = categoryKeys(shown, slot)
}

// categoryKeys is the categorical legend. Categories that share the last slot
// are named together, because a swatch labelled with one of them would say the
// others are something else.
func categoryKeys(categories []string, slot map[string]int) []MapKey {
	var out []MapKey
	var folded []string
	seen := map[string]bool{}
	for _, c := range categories {
		if seen[c] {
			continue
		}
		seen[c] = true
		if s := slot[c]; s < PlotSlots-1 {
			out = append(out, MapKey{Label: c, Slot: s})
		} else {
			folded = append(folded, c)
		}
	}
	switch len(folded) {
	case 0:
	case 1:
		out = append(out, MapKey{Label: folded[0], Slot: PlotSlots - 1})
	default:
		out = append(out, MapKey{Label: fmt.Sprintf("%s and %d more", folded[0], len(folded)-1),
			Slot: PlotSlots - 1})
	}
	return out
}

// markersDrawn reports whether any layer draws the points themselves.
func markersDrawn(m *definition.MapSpec) bool {
	for _, l := range []definition.MapLayer{definition.HeatLayer, definition.ClusterLayer,
		definition.BubbleLayer, definition.ScatterLayer, definition.RadiusLayer} {
		if m.Draws(l) {
			return true
		}
	}
	return false
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
// definition.Block.Folds.
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
func foldOf(m definition.MeasureRef, ds definition.Dataset) definition.Fold {
	name := m.Aggregate
	if name == "" {
		if f, ok := ds.Field(m.Field); ok {
			name = f.Aggregate
		}
	}
	return definition.Foldable(name)
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
