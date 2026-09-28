package run

// Tiles is the basemap a viewer draws under the data.
//
// Carried on the wire rather than configured in the viewer, because it is the
// report author who chose the tile source and the attribution that goes with
// it. A host page that wanted to substitute its own would be substituting the
// credit line too, which is the one part no tile source makes optional.
type Tiles struct {
	// URL is an XYZ template the viewer fills in per tile. Always a plain
	// {z}/{x}/{y} template, so a viewer that predates everything below still
	// draws a basemap from it.
	URL string `json:"url"`
	// URL2x is the same tiles at twice the pixel density, for a screen that
	// has it. Absent where the source has no such thing.
	URL2x string `json:"url2x,omitempty"`
	// Attribution is the credit line the viewer must display, as plain text.
	Attribution string `json:"attribution"`
	// Credits is the same line in parts, each linked where the provider's
	// terms want a link — "© OpenStreetMap" to the copyright page, Mapbox's
	// "Improve this map" to its feedback form. A viewer that knows this field
	// draws it instead of Attribution.
	Credits []Credit `json:"credits,omitempty"`
	// Logo is an image the provider requires be shown on the map itself, and
	// LogoAlt what it says. Google's and Mapbox's terms both ask for one.
	Logo    string `json:"logo,omitempty"`
	LogoAlt string `json:"logoAlt,omitempty"`
	// Viewport is where the viewer asks for the credit line of what is in
	// view, with the view appended as zoom, north, south, east and west.
	// Google's copyright depends on which imagery is on screen, so the line
	// the server sends is right for the map as first drawn and has to be
	// asked for again once the reader pans somewhere else.
	Viewport string `json:"viewport,omitempty"`
	MaxZoom  int    `json:"maxZoom"`
	// TileSize is the edge of a tile in CSS pixels at its native zoom: 256
	// for most sources, 512 for Mapbox's. The zoom to request follows from
	// it, so a viewer that assumed 256 asks for one level too deep — more
	// requests for the same picture, which is why it is sent.
	TileSize int `json:"tileSize,omitempty"`
	// Dark is the same provider's tiles for a dark page, when the author asked
	// for a basemap that follows the page's theme (style: auto). A viewer
	// draws these under a dark theme and the ones above under a light one; a
	// viewer that predates this field draws the light ones everywhere.
	Dark *Tiles `json:"dark,omitempty"`
}
