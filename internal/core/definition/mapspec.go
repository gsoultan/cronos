package definition

import "fmt"

// MapSpec is the geography a map block reads.
//
// Nested under `map:` rather than flattened into Block like the other kinds.
// Block's fields are a flat union because each kind adds two or three; a map
// adds nine, and folding those into the union would mean every author reading
// the block reference scrolls past `toLat` to find `columns`. One key whose
// value is a mapping is the smaller cost.
type MapSpec struct {
	// Layers to draw, bottom to top. Empty means polygon when a geometry
	// field is named and scatter when coordinates are.
	Layers []MapLayer `json:"layers,omitempty" yaml:"layers,omitempty"`

	// Geometry names the field carrying GeoJSON for the polygon layer —
	// ST_AsGeoJSON in PostGIS, ST_AsGeoJSON in DuckDB's spatial extension, or
	// a text column somebody stored it in.
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
}

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
	case l == PolygonLayer && m.Geometry == "":
		return fmt.Errorf("%w: %s map %d draws polygons but names no geometry field",
			ErrInvalid, output, i)
	case l.Points() && (m.Lat == "" || m.Lon == ""):
		return fmt.Errorf("%w: %s map %d draws a %s layer, which needs lat and lon",
			ErrInvalid, output, i, l)
	case l == FlowLayer && (m.ToLat == "" || m.ToLon == ""):
		return fmt.Errorf("%w: %s map %d draws flows, which need toLat and toLon "+
			"for where each one lands", ErrInvalid, output, i)
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
	if m.Draws(PolygonLayer) {
		out = append(out, GeometryCol)
	}
	if m.points() {
		out = append(out, LatCol, LonCol)
	}
	if m.Draws(FlowLayer) {
		out = append(out, ToLatCol, ToLonCol)
	}
	out = append(out, ValueCol)
	if b.Size.Field != "" {
		out = append(out, SizeCol)
	}
	// No series column: a map does not split by series — see run.Marker.
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

// Grouped reports whether the block runs at point grain while also shading
// polygons, so the polygon values have to be folded a second time.
//
// This is the one combination a single query cannot answer exactly: a row per
// point and a row per region are different grains, and a block compiles to one
// plan. Running at the finer grain and folding the coarser one from it is
// correct for sum, count, min and max, and wrong for avg — which is why
// validateFold refuses that rather than drawing an average of averages.
func (b Block) Grouped() bool {
	return b.Map != nil && b.Map.Draws(PolygonLayer) && b.Map.points()
}
