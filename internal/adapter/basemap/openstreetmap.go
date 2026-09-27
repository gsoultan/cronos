package basemap

import (
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
)

// openStreetMap is the OpenStreetMap Foundation's standard layer.
//
// No key and no session — and a usage policy with teeth: heavy use is
// forbidden, and a request with no Referer header is answered with a tile
// reading "Referer is required". The viewer sends the page's origin for that
// reason; see packages/charts/src/map/tiles.ts.
func openStreetMap(b definition.Basemap) *run.Tiles {
	return &run.Tiles{
		URL:         "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
		Attribution: "© OpenStreetMap contributors",
		Credits: []run.Credit{
			{Text: "© OpenStreetMap contributors", Href: "https://www.openstreetmap.org/copyright"},
			// Recommended rather than required by the attribution guidelines,
			// and the cheapest way a reader who spots a wrong street can fix it.
			{Text: "Report a map issue", Href: "https://www.openstreetmap.org/fixthemap"},
		},
		MaxZoom:  capped(b, 19),
		TileSize: 256,
	}
}

// capped is the provider's deepest zoom, or the author's when theirs is
// shallower. Never deeper: a zoom the provider does not serve is a map that
// goes blank under the reader's thumb.
func capped(b definition.Basemap, most int) int {
	if b.MaxZoom > 0 && b.MaxZoom < most {
		return b.MaxZoom
	}
	return most
}
