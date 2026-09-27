package run

import (
	"context"
	"fmt"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// Basemaps turns the basemap an author named into the tiles a viewer asks for.
//
// A port, because the answer involves things this package has no business
// holding: a provider's key, read from wherever the deployment keeps its
// secrets, and — for Google — a session minted over the network before the
// first tile can be requested. internal/adapter/basemap is the
// implementation. A Service without one draws URL basemaps as it always has
// and says so for the rest, which is templateTiles.
type Basemaps interface {
	// Tiles resolves b. around is the box the map fits, for a provider whose
	// credit line depends on what is in view. An Unavailable error carries a
	// sentence the reader can be shown; any other error is a fault, and the
	// reader is told only that the basemap is missing.
	Tiles(ctx context.Context, b definition.Basemap, around Bounds) (*Tiles, error)
}

// templateTiles is what a Service with no Basemaps can draw: a URL basemap
// that needs nothing resolved.
func templateTiles(b definition.Basemap) (*Tiles, error) {
	switch {
	case b.Provider != "":
		return nil, Unavailable{Reason: fmt.Sprintf(
			"This server is not set up for %s, so the map is drawn without its basemap.",
			b.Provider.Title())}
	case strings.Contains(b.URL, "${secret:"):
		return nil, Unavailable{Reason: "This server cannot read the key the basemap " +
			"needs, so the map is drawn without it."}
	}
	url, url2x := b.Templates()
	return &Tiles{
		URL: url, URL2x: url2x, Attribution: b.Attribution,
		MaxZoom: b.Zoom(), TileSize: 256,
	}, nil
}
