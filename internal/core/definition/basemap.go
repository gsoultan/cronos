package definition

import (
	"fmt"
	"net/url"
	"strings"
)

// Basemap is the tile layer drawn under a map's data.
//
// Opt-in, and empty by default. A basemap is a request from our customer's
// end user's browser to a third party that we chose for them: it discloses
// roughly where the data is to whoever serves the tiles, and on OpenStreetMap's
// own servers it is against the tile usage policy for anything at volume. An
// author who wants one names one.
//
// There is no provider enum. `{z}/{x}/{y}` is the same contract at OSM, at a
// self-hosted tileserver and at a commercial one, so a URL template covers all
// of them and does not need a code change to add the next.
type Basemap struct {
	// URL is an XYZ tile template, e.g.
	// https://tile.openstreetmap.org/{z}/{x}/{y}.png
	URL string `json:"url" yaml:"url"`
	// Attribution is the credit line the viewer must display. Required,
	// because every tile source worth using requires it and a viewer that
	// leaves it out puts our customer in breach rather than us.
	Attribution string `json:"attribution" yaml:"attribution"`
	// MaxZoom caps how far the viewer will request tiles. Zero means 19.
	MaxZoom int `json:"maxZoom,omitempty" yaml:"maxZoom,omitempty"`
}

// DefaultMaxZoom is as deep as a viewer zooms when the author did not say.
const DefaultMaxZoom = 19

// Zoom is the cap to actually use.
func (b Basemap) Zoom() int {
	if b.MaxZoom > 0 {
		return b.MaxZoom
	}
	return DefaultMaxZoom
}

// Validate reports why the basemap cannot be stored.
//
// The scheme check is the one that matters: an embedded report runs on our
// customer's HTTPS page, so an http:// tile template is mixed content that the
// browser blocks silently — a map that renders blank with nothing in the
// report saying why.
func (b Basemap) Validate(output string, i int) error {
	u, err := url.Parse(b.URL)
	switch {
	case b.URL == "":
		return fmt.Errorf("%w: %s map %d has a basemap with no url", ErrInvalid, output, i)
	case err != nil:
		return fmt.Errorf("%w: %s map %d basemap url %q does not parse: %s",
			ErrInvalid, output, i, b.URL, err)
	case u.Scheme != "https":
		return fmt.Errorf("%w: %s map %d basemap url %q must be https — an embedded "+
			"report is served over https and a browser blocks mixed-content tiles",
			ErrInvalid, output, i, b.URL)
	case !strings.Contains(b.URL, "{x}") ||
		!strings.Contains(b.URL, "{y}") ||
		!strings.Contains(b.URL, "{z}"):
		return fmt.Errorf("%w: %s map %d basemap url %q is not an xyz template — "+
			"it needs {z}, {x} and {y}", ErrInvalid, output, i, b.URL)
	case strings.TrimSpace(b.Attribution) == "":
		return fmt.Errorf("%w: %s map %d basemap has no attribution, which every "+
			"tile source requires be displayed", ErrInvalid, output, i)
	case b.MaxZoom < 0 || b.MaxZoom > 22:
		return fmt.Errorf("%w: %s map %d basemap maxZoom %d is outside 0–22",
			ErrInvalid, output, i, b.MaxZoom)
	}
	return nil
}
