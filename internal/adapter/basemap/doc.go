/*
Package basemap turns the basemap a report names into tiles a browser can draw.

It implements run.Basemaps. Everything a tile provider's terms ask for lives
here, in one place per provider, rather than in a URL an author types: the
template, the credit line in the provider's own words and links, the logo its
terms require on the map, the key, and — for Google — the session that has to
be minted before the first tile can be requested.

The tiles are requested by the reader's browser, not proxied through cronos.
Google's terms forbid caching or rehosting its tiles, CARTO's forbid proxying
them, and a tile proxy is a bandwidth bill and a latency hop for every pan. So
a key a basemap uses is sent to the browser by design — which is why the only
secrets this package resolves are the ones named as tile keys; see
definition.TileKeyPrefix.

A basemap that cannot be drawn is never an error that fails the report. The
map is drawn without it, the reader is told why in words that name no setting,
and the operator is told which setting in the log.
*/
package basemap
