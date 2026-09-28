package definition

// MapLayer is one thing drawn on a map, bottom to top in the order listed.
//
// Layers rather than a map chart type per combination, because the question
// authors actually ask is "shade the regions *and* put a dot on each depot",
// and a type per combination is a combinatorial list nobody can hold.
type MapLayer string

const (
	// PolygonLayer shades regions by value — a choropleth.
	PolygonLayer MapLayer = "polygon"
	// LineLayer draws routes, roads and pipelines, coloured by value.
	LineLayer MapLayer = "line"
	// HexbinLayer folds points into hexagons and shades each by the total of
	// the points inside it.
	HexbinLayer MapLayer = "hexbin"
	// HeatLayer spreads point values into a density field.
	HeatLayer MapLayer = "heat"
	// ClusterLayer gathers points that would overlap into one counted circle,
	// and separates them again as the reader zooms in.
	ClusterLayer MapLayer = "cluster"
	// BubbleLayer draws a circle per point, sized by measure.
	BubbleLayer MapLayer = "bubble"
	// ScatterLayer draws a fixed-size dot per point.
	ScatterLayer MapLayer = "scatter"
	// FlowLayer draws an arc from an origin to a destination.
	FlowLayer MapLayer = "flow"
	// RadiusLayer draws a circle of MapSpec.RadiusKm around each point — a
	// delivery area, a catchment — measured on the ground.
	RadiusLayer MapLayer = "radius"
	// H3Layer shades the cells of Uber's H3 grid: cells a warehouse has
	// indexed its rows by, in MapSpec.H3, or the places at lat and lon binned
	// into them — each cell by the total of what falls in it.
	H3Layer MapLayer = "h3"
)

var mapLayers = []MapLayer{
	PolygonLayer, LineLayer, HexbinLayer, HeatLayer, ClusterLayer,
	BubbleLayer, ScatterLayer, FlowLayer, RadiusLayer, H3Layer,
}

// Valid reports whether l is a layer the renderers implement.
func (l MapLayer) Valid() bool {
	for _, k := range mapLayers {
		if l == k {
			return true
		}
	}
	return false
}

// Points reports whether the layer reads a coordinate per row rather than a
// geometry. An H3 layer may read either cells or places — see MapSpec.points.
func (l MapLayer) Points() bool { return !l.Geometric() && l != H3Layer }

// Geometric reports whether the layer reads the GeoJSON field. A polygon
// layer draws the areas in it and a line layer the lines; one field serves
// both, so a column holding districts and the roads between them is one
// block rather than two.
func (l MapLayer) Geometric() bool { return l == PolygonLayer || l == LineLayer }

// Ramped reports whether the layer shades each mark from the sequential ramp,
// which is what the legend under a map explains.
func (l MapLayer) Ramped() bool {
	return l == PolygonLayer || l == LineLayer || l == HexbinLayer || l == H3Layer
}

// Coloured reports whether the layer already spends colour on magnitude — the
// ramped layers, and a heat field, whose intensity is the value.
//
// Colour is one channel, so a map can give it to the value or to a category
// and not both — see Block.validateMapSeries.
func (l MapLayer) Coloured() bool { return l.Ramped() || l == HeatLayer }

// MapLayerNames lists every valid layer, for an error message or a form.
func MapLayerNames() []string {
	out := make([]string, len(mapLayers))
	for i, l := range mapLayers {
		out[i] = string(l)
	}
	return out
}
