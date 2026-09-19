package run

// Tiles is the basemap a viewer draws under the data.
//
// Carried on the wire rather than configured in the viewer, because it is the
// report author who chose the tile source and the attribution that goes with
// it. A host page that wanted to substitute its own would be substituting the
// credit line too, which is the one part no tile source makes optional.
type Tiles struct {
	// URL is an XYZ template the viewer fills in per tile.
	URL string `json:"url"`
	// Attribution is the credit line the viewer must display.
	Attribution string `json:"attribution"`
	MaxZoom     int    `json:"maxZoom"`
}
