package definition

// MapLayer is one thing drawn on a map, bottom to top in the order listed.
//
// Layers rather than five map chart types, because the question authors
// actually ask is "shade the regions *and* put a dot on each depot", and a
// type per combination is a combinatorial list nobody can hold.
type MapLayer string

const (
	// PolygonLayer shades regions by value — a choropleth.
	PolygonLayer MapLayer = "polygon"
	// HeatLayer spreads point values into a density field.
	HeatLayer MapLayer = "heat"
	// BubbleLayer draws a circle per point, sized by measure.
	BubbleLayer MapLayer = "bubble"
	// ScatterLayer draws a fixed-size dot per point.
	ScatterLayer MapLayer = "scatter"
	// FlowLayer draws an arc from an origin to a destination.
	FlowLayer MapLayer = "flow"
)

var mapLayers = []MapLayer{PolygonLayer, HeatLayer, BubbleLayer, ScatterLayer, FlowLayer}

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
// geometry. Every layer but polygon does.
func (l MapLayer) Points() bool { return l != PolygonLayer }

// MapLayerNames lists every valid layer, for an error message or a form.
func MapLayerNames() []string {
	out := make([]string, len(mapLayers))
	for i, l := range mapLayers {
		out[i] = string(l)
	}
	return out
}
