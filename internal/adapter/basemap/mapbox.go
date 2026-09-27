package basemap

import (
	"net/url"
	"strings"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
)

// mapboxStyles is each named style's id at Mapbox.
//
// The classic styles, because they are what the Static Tiles API renders —
// Mapbox Standard is vector-only. Mapbox no longer develops these, so a new
// version is a line here rather than something an author has to track.
var mapboxStyles = map[string]string{
	"streets":           "mapbox/streets-v12",
	"outdoors":          "mapbox/outdoors-v12",
	"light":             "mapbox/light-v11",
	"dark":              "mapbox/dark-v11",
	"satellite":         "mapbox/satellite-v9",
	"satellite-streets": "mapbox/satellite-streets-v12",
	"navigation-day":    "mapbox/navigation-day-v1",
	"navigation-night":  "mapbox/navigation-night-v1",
}

// mapbox is Mapbox's raster tiles, rendered from a style.
//
// 512-pixel tiles, which is Mapbox's own default: one request covers what
// four 256-pixel tiles would, and Mapbox bills per request. The viewer is told
// the size, so it asks for the zoom a 512 tile is drawn at rather than one
// level too deep.
func mapbox(b definition.Basemap, token string) *run.Tiles {
	style := b.Styled()
	id, named := mapboxStyles[style]
	if !named {
		// owner/style from Mapbox Studio, which definition already checked
		// is the shape of one.
		id = style
	}
	base := "https://api.mapbox.com/styles/v1/" + id + "/tiles/512/{z}/{x}/{y}"
	query := "?access_token=" + url.QueryEscape(token)

	credits := []run.Credit{
		{Text: "© Mapbox", Href: "https://www.mapbox.com/about/maps"},
		{Text: "© OpenStreetMap", Href: "https://www.openstreetmap.org/copyright"},
	}
	if strings.HasPrefix(style, "satellite") {
		credits = append(credits, run.Credit{Text: "© Maxar", Href: "https://www.maxar.com/"})
	}
	credits = append(credits, run.Credit{Text: "Improve this map", Href: "https://apps.mapbox.com/feedback/"})

	return &run.Tiles{
		URL:         base + query,
		URL2x:       base + "@2x" + query,
		Attribution: plain(credits),
		Credits:     credits,
		Logo:        dataURI(mapboxLogo),
		LogoAlt:     "Mapbox",
		MaxZoom:     capped(b, 22),
		TileSize:    512,
	}
}

// plain is a credit line as one string, for a viewer that predates Credits.
func plain(credits []run.Credit) string {
	parts := make([]string, len(credits))
	for i, c := range credits {
		parts[i] = c.Text
	}
	return strings.Join(parts, " ")
}
