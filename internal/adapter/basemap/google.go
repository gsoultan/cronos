package basemap

import (
	"context"
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
	maxZoom int
}

// googleStyles is each named style's session. Terrain is a shaded relief
// that Google serves only under its roads layer, and hybrid is satellite with
// the same layer on top — so both carry layerRoadmap, which terrain requires.
var googleStyles = map[string]googleStyle{
	"roadmap":   {mapType: "roadmap", maxZoom: 21},
	"satellite": {mapType: "satellite", maxZoom: 20},
	"hybrid":    {mapType: "satellite", layers: []string{"layerRoadmap"}, maxZoom: 20},
	"terrain":   {mapType: "terrain", layers: []string{"layerRoadmap"}, maxZoom: 15},
}

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
	around run.Bounds) (*run.Tiles, error) {

	style := googleStyles[b.Styled()]
	req := sessionRequest{
		MapType: style.mapType, LayerTypes: style.layers,
		Language: or(b.Language, "en-US"), Region: strings.ToUpper(or(b.Region, "US")),
	}
	token, err := g.session(ctx, key, req, true)
	if err != nil {
		return nil, fmt.Errorf("minting a %s session: %w", b.Styled(), err)
	}
	// The same tiles at twice the pixels, for a screen that has them — a
	// second session, because density is fixed when one is minted. Minted
	// behind the reader rather than in front of them: the first render after
	// a restart draws the ordinary tiles, and the next has the sharp ones.
	// Waiting for both put two round trips to Google ahead of the first map
	// anybody opened; failing to get the second costs sharpness, not the map.
	hi := req
	hi.Scale, hi.HighDPI = "scaleFactor2x", true
	token2x, err := g.session(ctx, key, hi, false)
	if err != nil {
		token2x = ""
	}

	maxZoom := capped(b, style.maxZoom)
	line := g.copyright(ctx, token, key, around, maxZoom)
	out := &run.Tiles{
		URL:         g.tileURL(token, key),
		Attribution: line,
		Credits:     []run.Credit{{Text: line}},
		Logo:        dataURI(googleLogo(style)),
		LogoAlt:     "Google Maps",
		MaxZoom:     maxZoom,
		TileSize:    256,
		Viewport: g.base + "/tile/v1/viewport?session=" + url.QueryEscape(token) +
			"&key=" + url.QueryEscape(key),
	}
	if token2x != "" {
		out.URL2x = g.tileURL(token2x, key)
	}
	return out, nil
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
	if s.mapType == "satellite" {
		return googleDark
	}
	return googleLight
}
