package definition

import "fmt"

// MapSpec is the geography a map block reads.
//
// Nested under `map:` rather than flattened into Block like the other kinds.
// Block's fields are a flat union because each kind adds two or three; a map
// adds a dozen, and folding those into the union would mean every author
// reading the block reference scrolls past `toLat` to find `columns`. One key
// whose value is a mapping is the smaller cost.
type MapSpec struct {
	// Layers to draw, bottom to top. Empty means polygon when a geometry
	// field is named and scatter when coordinates are.
	Layers []MapLayer `json:"layers,omitempty" yaml:"layers,omitempty"`

	// Geometry names the field carrying GeoJSON for the polygon and line
	// layers — ST_AsGeoJSON in PostGIS, ST_AsGeoJSON in DuckDB's spatial
	// extension, or a text column somebody stored it in.
	Geometry string `json:"geometry,omitempty" yaml:"geometry,omitempty"`
	// Region labels a polygon. Empty falls back to the block's x.
	Region string `json:"region,omitempty" yaml:"region,omitempty"`

	// Lat and Lon name the coordinate fields every point layer reads.
	Lat string `json:"lat,omitempty" yaml:"lat,omitempty"`
	Lon string `json:"lon,omitempty" yaml:"lon,omitempty"`
	// ToLat and ToLon are a flow layer's destination.
	ToLat string `json:"toLat,omitempty" yaml:"toLat,omitempty"`
	ToLon string `json:"toLon,omitempty" yaml:"toLon,omitempty"`

	// Basemap opts into third-party tiles under the data. Nil draws none.
	Basemap *Basemap `json:"basemap,omitempty" yaml:"basemap,omitempty"`

	// Simplify is the Douglas-Peucker tolerance in Web Mercator world units,
	// where the whole world is 1.0. Zero means DefaultSimplify. A negative
	// value keeps every vertex — correct, and a way to put six megabytes of
	// coastline through a browser, so it is spelled rather than defaulted.
	Simplify float64 `json:"simplify,omitempty" yaml:"simplify,omitempty"`

	// Overlays draw other datasets on the same map, over this one's layers:
	// the depots over the deliveries they serve, the zones under both. Each
	// is a map block of its own — a dataset, a label, a measure and layers —
	// without a basemap, which the map under it already has, or overlays of
	// its own.
	Overlays []Block `json:"overlays,omitempty" yaml:"overlays,omitempty"`

	// HexKm is how wide a hexbin layer's hexagons are, flat side to flat
	// side, in kilometres — to within a few percent, see run.hexRadius. Zero
	// sizes them to the data, about two dozen across. A width in kilometres
	// rather than a count, because the count would redraw the grid every time
	// a filter moved the edge of the data — and a hexagon that changes size
	// when somebody filters by carrier is a number that cannot be compared
	// with the one before it.
	HexKm float64 `json:"hexKm,omitempty" yaml:"hexKm,omitempty"`
}

// MaxOverlays is how many datasets a map may draw over its own. Each is a
// query of its own, and a map of more than five things is a legend nobody
// can read.
const MaxOverlays = 4

// MaxHexKm is as wide as a hexagon may be. Wider than a continent is a map
// with one hexagon on it, which is a stat tile drawn expensively.
const MaxHexKm = 5000

// DefaultSimplify drops vertices closer than roughly 40 metres at the equator.
//
// A national boundary carries tens of thousands of points because it was
// digitised for cartography, and a report is read at a few hundred pixels
// across. Sending the untouched ring means a payload two orders of magnitude
// larger than the numbers it is there to colour.
const DefaultSimplify = 0.000001

// Tolerance is the simplification to actually apply.
func (m MapSpec) Tolerance() float64 {
	switch {
	case m.Simplify < 0:
		return 0
	case m.Simplify == 0:
		return DefaultSimplify
	}
	return m.Simplify
}

// Draws reports whether the spec asks for layer l.
func (m MapSpec) Draws(l MapLayer) bool {
	for _, k := range m.Resolved() {
		if k == l {
			return true
		}
	}
	return false
}

// Resolved is Layers, or what the named fields imply when the author left it
// empty. A spec that names a geometry means a choropleth; one that names
// coordinates means dots.
func (m MapSpec) Resolved() []MapLayer {
	if len(m.Layers) > 0 {
		return m.Layers
	}
	if m.Geometry != "" {
		return []MapLayer{PolygonLayer}
	}
	if m.Lat != "" && m.Lon != "" {
		return []MapLayer{ScatterLayer}
	}
	return nil
}

// Validate reports why the map cannot be stored.
func (m MapSpec) Validate(output string, i int) error {
	layers := m.Resolved()
	if len(layers) == 0 {
		return fmt.Errorf("%w: %s map %d draws nothing — name a geometry field, "+
			"lat and lon, or the layers to draw", ErrInvalid, output, i)
	}
	for _, l := range layers {
		if err := m.validateLayer(output, i, l); err != nil {
			return err
		}
	}
	if err := m.validateHexes(output, i); err != nil {
		return err
	}
	if m.Basemap != nil {
		return m.Basemap.Validate(output, i)
	}
	return nil
}

func (m MapSpec) validateLayer(output string, i int, l MapLayer) error {
	switch {
	case !l.Valid():
		return fmt.Errorf("%w: %s map %d has layer %q, want one of %v",
			ErrInvalid, output, i, l, MapLayerNames())
	case l.Geometric() && m.Geometry == "":
		return fmt.Errorf("%w: %s map %d draws a %s layer but names no geometry field",
			ErrInvalid, output, i, l)
	case l.Points() && (m.Lat == "" || m.Lon == ""):
		return fmt.Errorf("%w: %s map %d draws a %s layer, which needs lat and lon",
			ErrInvalid, output, i, l)
	case l == FlowLayer && (m.ToLat == "" || m.ToLon == ""):
		return fmt.Errorf("%w: %s map %d draws flows, which need toLat and toLon "+
			"for where each one lands", ErrInvalid, output, i)
	}
	return nil
}

// validateHexes checks what a hexbin layer is combined with and how wide it
// draws.
func (m MapSpec) validateHexes(output string, i int) error {
	hexes := m.Draws(HexbinLayer)
	switch {
	case hexes && m.geometric():
		// Both shade from the ramp, from different rows: a region's value is
		// its own row and a hexagon's is the points inside it. One legend
		// cannot explain two scales, and two legends under one map is a
		// reader matching colours to the wrong one.
		return fmt.Errorf("%w: %s map %d shades hexagons and draws the geometry "+
			"field too, and one legend cannot explain both — split them across two "+
			"blocks", ErrInvalid, output, i)
	case m.HexKm < 0 || m.HexKm > MaxHexKm:
		return fmt.Errorf("%w: %s map %d hexKm %g is outside 0–%d",
			ErrInvalid, output, i, m.HexKm, MaxHexKm)
	case m.HexKm > 0 && !hexes:
		return fmt.Errorf("%w: %s map %d sets hexKm but draws no hexbin layer",
			ErrInvalid, output, i)
	}
	return nil
}

// MapColumn names one column of a map block's result.
type MapColumn string

const (
	RegionCol   MapColumn = "region"
	GeometryCol MapColumn = "geometry"
	LatCol      MapColumn = "lat"
	LonCol      MapColumn = "lon"
	ToLatCol    MapColumn = "toLat"
	ToLonCol    MapColumn = "toLon"
	SeriesCol   MapColumn = "series"
	ValueCol    MapColumn = "value"
	SizeCol     MapColumn = "size"
)

// MapColumns is the result shape a map block's layers imply, in order.
//
// One function, called by the compiler that writes the SELECT and by the
// reader that scans it back. They were going to agree by both being edited
// together, which is the kind of agreement that lasts until the first time
// only one of them is.
func (b Block) MapColumns() []MapColumn {
	if b.Map == nil {
		return nil
	}
	m := b.Map
	var out []MapColumn
	// The region labels whatever the row is — the polygon it shades and the
	// dot it places alike. A point layer with nothing to call its dots leaves
	// every tooltip reading a pair of coordinates, which is a location and not
	// an answer.
	if b.Labels() != "" {
		out = append(out, RegionCol)
	}
	if m.geometric() {
		out = append(out, GeometryCol)
	}
	if m.points() {
		out = append(out, LatCol, LonCol)
	}
	if m.Draws(FlowLayer) {
		out = append(out, ToLatCol, ToLonCol)
	}
	// The category a point is coloured by. Only on the layers that leave
	// colour free for it — see validateMapSeries.
	if b.Series.Field != "" {
		out = append(out, SeriesCol)
	}
	out = append(out, ValueCol)
	if b.Size.Field != "" {
		out = append(out, SizeCol)
	}
	return out
}

// Labels is the field naming what each row of a map is, or empty for none.
//
// map.region wins over x so a block can group by one field and label with
// another — group by country code, label with country name.
func (b Block) Labels() string {
	if b.Map != nil && b.Map.Region != "" {
		return b.Map.Region
	}
	return b.X.Field
}

// points reports whether any resolved layer reads a coordinate.
func (m MapSpec) points() bool {
	for _, l := range m.Resolved() {
		if l.Points() {
			return true
		}
	}
	return false
}

// geometric reports whether any resolved layer reads the geometry field.
func (m MapSpec) geometric() bool {
	for _, l := range m.Resolved() {
		if l.Geometric() {
			return true
		}
	}
	return false
}

// Grouped reports whether the block runs at point grain while also shading
// polygons, so the polygon values have to be folded a second time.
//
// This is the one combination a single query cannot answer exactly: a row per
// point and a row per region are different grains, and a block compiles to one
// plan. Running at the finer grain and folding the coarser one from it is
// correct for sum, count, min and max, and wrong for avg — which is why
// validateFold refuses that rather than drawing an average of averages.
func (b Block) Grouped() bool {
	return b.Map != nil && b.Map.geometric() && b.Map.points()
}

// Folds reports whether any of the block's values is its measure applied a
// second time: a region added up from the points under it, or a hexagon from
// the points inside it. Either way the aggregate has to survive being applied
// to its own results, which an average does not.
func (b Block) Folds() bool {
	return b.Grouped() || (b.Map != nil && b.Map.Draws(HexbinLayer))
}

/*
OverlaysFor is the block's overlays as blocks a renderer can run: each a map
chart, reading the dataset it names or, naming none, the one the map under it
reads — an overlay of the same rows drawn another way needs no dataset of its
own.
*/
func (b Block) OverlaysFor(reportDefault string) []Block {
	if b.Map == nil || len(b.Map.Overlays) == 0 {
		return nil
	}
	out := make([]Block, len(b.Map.Overlays))
	for i, ov := range b.Map.Overlays {
		ov.Kind, ov.Chart = ChartBlock, MapChart
		if ov.Dataset == "" {
			ov.Dataset = b.DatasetFor(reportDefault)
		}
		out[i] = ov
	}
	return out
}
