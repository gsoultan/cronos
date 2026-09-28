package basemap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
)

// googleBase is the Map Tiles API.
const googleBase = "https://tile.googleapis.com"

// googleStyle is what a style asks createSession for.
type googleStyle struct {
	mapType string
	layers  []string
	// styles recolour the map — Google's own style rules, as its JavaScript
	// API takes them — and look names them, for the session they are minted
	// into: a styled map is a session of its own.
	styles  json.RawMessage
	look    string
	dark    bool
	maxZoom int
}

// googleStyles is each named style's session. Terrain is a shaded relief
// that Google serves only under its roads layer, and hybrid is satellite with
// the same layer on top — so both carry layerRoadmap, which terrain requires.
// Dark is the roadmap in night colours: Google draws no dark map of its own.
var googleStyles = map[string]googleStyle{
	"roadmap":   {mapType: "roadmap", maxZoom: 21},
	"satellite": {mapType: "satellite", maxZoom: 20},
	"hybrid":    {mapType: "satellite", layers: []string{"layerRoadmap"}, maxZoom: 20},
	"terrain":   {mapType: "terrain", layers: []string{"layerRoadmap"}, maxZoom: 15},
	"dark":      {mapType: "roadmap", styles: nightStyles, look: "night", dark: true, maxZoom: 21},
}

// nightStyles are Google's own night colours for its roadmap, from its
// styling documentation: land and water dark, labels and roads muted to sit
// under data rather than over it.
var nightStyles = json.RawMessage(`[
{"elementType":"geometry","stylers":[{"color":"#242f3e"}]},
{"elementType":"labels.text.stroke","stylers":[{"color":"#242f3e"}]},
{"elementType":"labels.text.fill","stylers":[{"color":"#746855"}]},
{"featureType":"administrative.locality","elementType":"labels.text.fill","stylers":[{"color":"#d59563"}]},
{"featureType":"poi","elementType":"labels.text.fill","stylers":[{"color":"#d59563"}]},
{"featureType":"poi.park","elementType":"geometry","stylers":[{"color":"#263c3f"}]},
{"featureType":"poi.park","elementType":"labels.text.fill","stylers":[{"color":"#6b9a76"}]},
{"featureType":"road","elementType":"geometry","stylers":[{"color":"#38414e"}]},
{"featureType":"road","elementType":"geometry.stroke","stylers":[{"color":"#212a37"}]},
{"featureType":"road","elementType":"labels.text.fill","stylers":[{"color":"#9ca5b3"}]},
{"featureType":"road.highway","elementType":"geometry","stylers":[{"color":"#746855"}]},
{"featureType":"road.highway","elementType":"geometry.stroke","stylers":[{"color":"#1f2835"}]},
{"featureType":"road.highway","elementType":"labels.text.fill","stylers":[{"color":"#f3d19c"}]},
{"featureType":"transit","elementType":"geometry","stylers":[{"color":"#2f3948"}]},
{"featureType":"transit.station","elementType":"labels.text.fill","stylers":[{"color":"#d59563"}]},
{"featureType":"water","elementType":"geometry","stylers":[{"color":"#17263c"}]},
{"featureType":"water","elementType":"labels.text.fill","stylers":[{"color":"#515c6d"}]},
{"featureType":"water","elementType":"labels.text.stroke","stylers":[{"color":"#17263c"}]}
]`)

// googleTiles mints and keeps the sessions Google's 2D tiles are asked for
// with, and the copyright line each view carries.
//
// A session is valid for about two weeks and may be shared by every reader,
// so one per key, style, locale and pixel density serves the whole
// deployment; minting per render would be a round trip to Google in front of
// every map anybody opened.
type googleTiles struct {
	client *http.Client
	base   string
	now    func() time.Time

	mu       sync.Mutex
	sessions map[sessionKey]*session
	credits  map[creditKey]credit
}

func newGoogle(client *http.Client, base string) *googleTiles {
	return &googleTiles{
		client: client, base: base, now: time.Now,
		sessions: map[sessionKey]*session{}, credits: map[creditKey]credit{},
	}
}

// tiles is a Google basemap, with a session minted if there is not one.
func (g *googleTiles) tiles(ctx context.Context, b definition.Basemap, key string,
	around run.Bounds, wait bool) (*run.Tiles, error) {

	style := googleStyles[b.Styled()]
	req := sessionRequest{
		MapType: style.mapType, LayerTypes: style.layers, Styles: style.styles, look: style.look,
		Language: or(b.Language, "en-US"), Region: strings.ToUpper(or(b.Region, "US")),
	}
	token, err := g.session(ctx, key, req, wait)
	if errors.Is(err, errPending) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("minting a %s session: %w", b.Styled(), err)
	}
	token2x := g.sharper(ctx, key, req)
	maxZoom := capped(b, style.maxZoom)
	// A caller that would not wait for a session would not wait for its
	// credit line either; it has the line from the map it already holds.
	line := ""
	if wait {
		line = g.copyright(ctx, token, key, around, maxZoom)
	}
	out := &run.Tiles{
		URL:         g.tileURL(token, key),
		Attribution: line,
		Logo:        dataURI(googleLogo(style)),
		LogoAlt:     "Google Maps",
		MaxZoom:     maxZoom,
		TileSize:    256,
		Viewport: g.base + "/tile/v1/viewport?session=" + url.QueryEscape(token) +
			"&key=" + url.QueryEscape(key),
	}
	if line != "" {
		out.Credits = []run.Credit{{Text: line}}
	}
	if token2x != "" {
		out.URL2x = g.tileURL(token2x, key)
	}
	return out, nil
}

/*
sharper is the session for the same tiles at twice the pixels, for a screen
that has them — a second session, because density is fixed when one is minted
— or empty while it is minted. Minted behind the reader rather than in front of
them: the first render after a restart draws the ordinary tiles, and the next
has the sharp ones. Waiting for both put two round trips to Google ahead of the
first map anybody opened; failing to get the second costs sharpness, not the map.
*/
func (g *googleTiles) sharper(ctx context.Context, key string, req sessionRequest) string {
	hi := req
	hi.Scale, hi.HighDPI = "scaleFactor2x", true
	token, err := g.session(ctx, key, hi, false)
	if err != nil {
		return ""
	}
	return token
}

func (g *googleTiles) tileURL(token, key string) string {
	return g.base + "/v1/2dtiles/{z}/{x}/{y}?session=" + url.QueryEscape(token) +
		"&key=" + url.QueryEscape(key)
}

// fitZoom is the zoom the viewer will first draw bounds at, for a panel of a
// typical width. Only the copyright line depends on it, and that is asked for
// again by the viewer as soon as the real width is known.
func fitZoom(b run.Bounds, most int) int {
	span := math.Max(b.MaxX-b.MinX, 1e-9)
	z := int(math.Round(math.Log2(800 / (span * 256))))
	return max(0, min(most, z))
}

func or(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

// googleLogo is the wordmark for the ground under it: dark with a light
// outline on a drawn map, white with a dark outline on imagery, where the
// dark one disappears into a forest.
func googleLogo(s googleStyle) string {
	if s.mapType == "satellite" || s.dark {
		return googleDark
	}
	return googleLight
}
