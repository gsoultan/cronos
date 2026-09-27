package definition

import "regexp"

// BasemapProvider names a tile source whose terms cronos knows.
//
// A closed set, like ChartType. What a provider needs is more than a URL —
// Google mints a session before it serves a tile, Mapbox bills a token per
// request, and each requires a logo and a credit line in its own wording — so
// adding one is code in internal/adapter/basemap rather than a string an
// author can make up.
type BasemapProvider string

const (
	// OpenStreetMap is the OpenStreetMap Foundation's standard tile layer.
	// No key, and a usage policy that forbids heavy use: right for a demo and
	// a low-traffic internal report, wrong for an embedded product at volume.
	OpenStreetMap BasemapProvider = "openstreetmap"
	// Mapbox is Mapbox's raster tiles, rendered from one of its styles or
	// from a style somebody made in Mapbox Studio.
	Mapbox BasemapProvider = "mapbox"
	// GoogleMaps is the Google Maps Platform Map Tiles API.
	GoogleMaps BasemapProvider = "google"
)

var basemapProviders = []BasemapProvider{OpenStreetMap, Mapbox, GoogleMaps}

// providerStyles is each provider's styles, default first.
var providerStyles = map[BasemapProvider][]string{
	OpenStreetMap: {"standard"},
	Mapbox: {
		"streets", "outdoors", "light", "dark", "satellite", "satellite-streets",
		"navigation-day", "navigation-night",
	},
	GoogleMaps: {"roadmap", "satellite", "terrain", "hybrid"},
}

// studioStyle is a Mapbox Studio style id, owner/style.
var studioStyle = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*/[A-Za-z0-9_-]+$`)

// Valid reports whether p is a provider cronos can draw.
func (p BasemapProvider) Valid() bool {
	_, ok := providerStyles[p]
	return ok
}

// Title is the provider's name as its owner writes it, for a sentence.
func (p BasemapProvider) Title() string {
	switch p {
	case OpenStreetMap:
		return "OpenStreetMap"
	case Mapbox:
		return "Mapbox"
	case GoogleMaps:
		return "Google Maps"
	}
	return string(p)
}

// DefaultStyle is what the provider draws when the author names no style.
func (p BasemapProvider) DefaultStyle() string {
	if s := providerStyles[p]; len(s) > 0 {
		return s[0]
	}
	return ""
}

// Styles reports whether the provider has style s.
func (p BasemapProvider) Styles(s string) bool {
	for _, k := range providerStyles[p] {
		if s == k {
			return true
		}
	}
	return p == Mapbox && studioStyle.MatchString(s)
}

// StyleNames lists the provider's named styles, for an error message or a
// form.
func (p BasemapProvider) StyleNames() []string {
	return append([]string(nil), providerStyles[p]...)
}

// DefaultKey is the secret a provider's key is read from when a basemap does
// not name one — CRONOS_SECRET_MAPBOX_TOKEN and CRONOS_SECRET_GOOGLE_MAPS_KEY
// in an environment. Empty for a provider that takes no key.
//
// One name per provider rather than a setting per report, because a
// deployment has one Mapbox account far more often than it has several, and
// "set this variable" is the whole of the instructions that way.
func (p BasemapProvider) DefaultKey() string {
	switch p {
	case Mapbox:
		return "mapbox-token"
	case GoogleMaps:
		return "google-maps-key"
	}
	return ""
}

// KeyPrefix is how the secret behind the provider's key must be named — see
// TileKeyPrefix for why a basemap cannot name any secret it likes.
func (p BasemapProvider) KeyPrefix() string {
	if p.DefaultKey() == "" {
		return ""
	}
	return string(p) + "-"
}

// BasemapProviderNames lists every provider, for an error message or a form.
func BasemapProviderNames() []string {
	out := make([]string, len(basemapProviders))
	for i, p := range basemapProviders {
		out[i] = string(p)
	}
	return out
}
