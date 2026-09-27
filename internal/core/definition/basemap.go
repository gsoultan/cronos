package definition

import (
	"fmt"
	"net/url"
	"regexp"
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
// Two forms, and exactly one of them. A provider — `openstreetmap`, `mapbox`,
// `google` — is a tile source whose terms cronos knows: its URL, the credit
// line and logo its licence requires, and where its key comes from. A URL is
// everything else: a self-hosted tileserver or a commercial one cronos has no
// name for, which `{z}/{x}/{y}` already describes without a code change.
//
// The providers were a URL template each until they were named, and the
// template was the easy part. Google's tiles need a session minted with the
// key before the first one can be asked for, Mapbox's need the key on every
// request, and both require a logo and a credit line whose wording is theirs —
// three things an author typing a URL had no way to get right.
type Basemap struct {
	// Provider names a tile source cronos knows the terms of.
	Provider BasemapProvider `json:"provider,omitempty" yaml:"provider,omitempty"`
	// Style is one of the provider's looks — `dark` at Mapbox, `satellite` at
	// Google. Empty means the provider's default. Mapbox also takes the id of
	// a style somebody made in Mapbox Studio, as `owner/style`.
	Style string `json:"style,omitempty" yaml:"style,omitempty"`
	// Key is a ${secret:name} reference to the provider's access key, for a
	// report that bills a different account from the deployment's. Empty means
	// the provider's own name — see DefaultKey.
	//
	// A reference and never the key. A definition is a file somebody commits,
	// and a Mapbox token pasted into one is in their git history for ever,
	// whatever Mapbox calls it.
	Key string `json:"key,omitempty" yaml:"key,omitempty"`
	// Language and Region localise Google's labels: a BCP 47 tag and a
	// two-letter region. Empty means en-US and US.
	Language string `json:"language,omitempty" yaml:"language,omitempty"`
	Region   string `json:"region,omitempty" yaml:"region,omitempty"`

	// URL is an XYZ tile template, e.g.
	// https://tiles.example.org/{z}/{x}/{y}.png
	//
	// `{r}` is filled with `@2x` on a high-density screen and with nothing
	// elsewhere, which is how most commercial tile servers spell retina.
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
	// Attribution is the credit line the viewer must display. Required with a
	// URL, because every tile source worth using requires it and a viewer that
	// leaves it out puts our customer in breach rather than us. Refused with a
	// provider, whose wording is set by its terms and written by cronos.
	Attribution string `json:"attribution,omitempty" yaml:"attribution,omitempty"`
	// MaxZoom caps how far the viewer will request tiles. Zero means the
	// provider's own limit, or 19 for a URL. A cap below the provider's is a
	// legitimate cost control: every zoom level is four times the tiles.
	MaxZoom int `json:"maxZoom,omitempty" yaml:"maxZoom,omitempty"`
}

// DefaultMaxZoom is as deep as a viewer zooms when the author did not say.
const DefaultMaxZoom = 19

// Zoom is the cap to actually use for a URL basemap.
func (b Basemap) Zoom() int {
	if b.MaxZoom > 0 {
		return b.MaxZoom
	}
	return DefaultMaxZoom
}

// Templates is a URL basemap's template at ordinary and at double pixel
// density. `{r}` becomes nothing in the first and `@2x` in the second; a
// template without it has no second, and that is reported as empty.
func (b Basemap) Templates() (url, url2x string) {
	if !strings.Contains(b.URL, "{r}") {
		return b.URL, ""
	}
	return strings.ReplaceAll(b.URL, "{r}", ""), strings.ReplaceAll(b.URL, "{r}", "@2x")
}

// Styled is the style to actually draw: the author's, or the provider's
// default.
func (b Basemap) Styled() string {
	if b.Style != "" {
		return b.Style
	}
	return b.Provider.DefaultStyle()
}

// KeyRef is the reference the provider's key is resolved from: the author's,
// or the one every deployment is told to set.
func (b Basemap) KeyRef() string {
	if b.Key != "" {
		return b.Key
	}
	if name := b.Provider.DefaultKey(); name != "" {
		return "${secret:" + name + "}"
	}
	return ""
}

// secretRef is a whole ${secret:name} reference and nothing else.
//
// The same name grammar internal/platform/secret resolves, restated here
// because this package sits below that one and cannot ask it. Anchored, so a
// key with a reference in the middle of a literal — `pk.${secret:x}` — is
// refused rather than stored as half a secret.
var secretRef = regexp.MustCompile(`^\$\{secret:([A-Za-z0-9_.-]+)\}$`)

// anyRef finds every reference in a string, for a URL that carries one among
// its query parameters.
var anyRef = regexp.MustCompile(`\$\{secret:([A-Za-z0-9_.-]*)\}`)

// TileKeyPrefix is how the secret behind a URL basemap's key must be named.
//
// A tile key is public by construction: it is in every tile request, and a
// reader who opens the network tab has it. So the secret a basemap names is
// sent to every browser that draws the map — and without a rule about which
// secrets those are, `${secret:warehouse-password}` in a tile URL sends the
// database password there too. The rule is the name. A secret called
// `tiles-…`, `mapbox-…` or `google-…` is one somebody made to be a tile key;
// anything else is refused before it is resolved.
const TileKeyPrefix = "tiles-"

var (
	languageTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8}){0,2}$`)
	regionCode  = regexp.MustCompile(`^[A-Za-z]{2}$`)
)

// Validate reports why the basemap cannot be stored.
func (b Basemap) Validate(output string, i int) error {
	switch {
	case b.Provider != "" && b.URL != "":
		return fmt.Errorf("%w: %s map %d names a basemap provider and a url — a "+
			"provider brings its own tiles, so name one or the other", ErrInvalid, output, i)
	case b.MaxZoom < 0 || b.MaxZoom > 22:
		return fmt.Errorf("%w: %s map %d basemap maxZoom %d is outside 0–22",
			ErrInvalid, output, i, b.MaxZoom)
	case b.Provider != "":
		return b.validateProvider(output, i)
	}
	return b.validateTemplate(output, i)
}

// validateProvider checks a named tile source.
func (b Basemap) validateProvider(output string, i int) error {
	switch {
	case !b.Provider.Valid():
		return fmt.Errorf("%w: %s map %d has basemap provider %q, want one of %v "+
			"or a url", ErrInvalid, output, i, b.Provider, BasemapProviderNames())
	case !b.Provider.Styles(b.Styled()):
		return fmt.Errorf("%w: %s map %d asks %s for style %q, want one of %v",
			ErrInvalid, output, i, b.Provider, b.Style, b.Provider.StyleNames())
	case b.Attribution != "":
		return fmt.Errorf("%w: %s map %d sets an attribution for %s, whose credit "+
			"line is set by its terms — cronos writes it", ErrInvalid, output, i, b.Provider)
	case b.Key != "" && b.Provider.DefaultKey() == "":
		return fmt.Errorf("%w: %s map %d sets a key for %s, which takes none",
			ErrInvalid, output, i, b.Provider)
	case b.Key != "" && !secretRef.MatchString(b.Key):
		return fmt.Errorf("%w: %s map %d basemap key must be a ${secret:name} "+
			"reference — a key written into a definition is in its history for ever",
			ErrInvalid, output, i)
	case b.Key != "" && !strings.HasPrefix(secretRef.FindStringSubmatch(b.Key)[1],
		b.Provider.KeyPrefix()):
		return fmt.Errorf("%w: %s map %d names secret %q as its %s key, and a key "+
			"reaches every reader's browser — only a secret named %s… is sent there",
			ErrInvalid, output, i, secretRef.FindStringSubmatch(b.Key)[1],
			b.Provider.Title(), b.Provider.KeyPrefix())
	}
	return b.validateLocale(output, i)
}

// validateLocale checks the two settings only Google reads.
func (b Basemap) validateLocale(output string, i int) error {
	if (b.Language != "" || b.Region != "") && b.Provider != GoogleMaps {
		// Mapbox bakes its language into the style and OpenStreetMap draws
		// each place in its own. Accepting the setting would promise a
		// translation nothing is going to make.
		return fmt.Errorf("%w: %s map %d sets a language or region, which only "+
			"the %s basemap reads", ErrInvalid, output, i, GoogleMaps)
	}
	switch {
	case b.Language != "" && !languageTag.MatchString(b.Language):
		return fmt.Errorf("%w: %s map %d basemap language %q is not a language "+
			"tag like en-US or id", ErrInvalid, output, i, b.Language)
	case b.Region != "" && !regionCode.MatchString(b.Region):
		return fmt.Errorf("%w: %s map %d basemap region %q is not a two-letter "+
			"region like US or ID", ErrInvalid, output, i, b.Region)
	}
	return nil
}

// validateTemplate checks a URL basemap.
//
// The scheme check is the one that matters: an embedded report runs on our
// customer's HTTPS page, so an http:// tile template is mixed content that the
// browser blocks silently — a map that renders blank with nothing in the
// report saying why.
func (b Basemap) validateTemplate(output string, i int) error {
	u, err := url.Parse(b.URL)
	switch {
	case b.URL == "":
		return fmt.Errorf("%w: %s map %d has a basemap with no provider and no url",
			ErrInvalid, output, i)
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
	case b.Style != "" || b.Key != "" || b.Language != "" || b.Region != "":
		// Each of these means something only to a provider cronos knows. On a
		// URL they would be stored, shown in the builder, and do nothing.
		return fmt.Errorf("%w: %s map %d basemap sets a style, key, language or "+
			"region, which only a named provider reads — put the key in the url "+
			"as ${secret:%sname} if the tile server needs one",
			ErrInvalid, output, i, TileKeyPrefix)
	}
	for _, m := range anyRef.FindAllStringSubmatch(b.URL, -1) {
		if !strings.HasPrefix(m[1], TileKeyPrefix) {
			return fmt.Errorf("%w: %s map %d basemap url names secret %q, and the url "+
				"reaches every reader's browser — only a secret named %s… is sent there",
				ErrInvalid, output, i, m[1], TileKeyPrefix)
		}
	}
	return nil
}
