package basemap

import (
	_ "embed"
	"strings"
)

// The logos each provider's terms require on the map. See logos/NOTICE.md
// for where they came from and why they may not be edited here.
var (
	//go:embed logos/google-maps-light-outline.svg
	googleLight string
	//go:embed logos/google-maps-dark-outline.svg
	googleDark string
	//go:embed logos/mapbox.svg
	mapboxLogo string
)

// dataURI makes an SVG an image source that needs no second request.
//
// Inline rather than served, because the viewer runs on our customer's page
// and has no route back to a cronos asset it could rely on — the payload is
// the one thing that always arrives. Percent-encoded rather than base64: an
// SVG is text, and only the characters that mean something in a URL need
// escaping, which keeps a logo about a third smaller than base64 would.
func dataURI(svg string) string {
	var b strings.Builder
	b.WriteString("data:image/svg+xml,")
	for _, r := range strings.ReplaceAll(svg, `"`, "'") {
		switch r {
		case '%', '#', '<', '>', '{', '}', '|', '\\', '^', '`', ' ', '\n', '\r', '\t':
			b.WriteString("%")
			b.WriteString(strings.ToUpper(hex2(byte(r))))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hex2(c byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[c>>4], digits[c&0x0f]})
}
