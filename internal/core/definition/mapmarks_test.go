package definition

import (
	"errors"
	"strings"
	"testing"
)

func TestAMapAddsToItsMarks(t *testing.T) {
	for name, edit := range map[string]func(*Block){
		"catchments": func(b *Block) { b.Map.Layers, b.Map.RadiusKm = []MapLayer{RadiusLayer, ScatterLayer}, 25 },
		"names":      func(b *Block) { b.Map.Labels = true },
		"moving flows": func(b *Block) {
			b.Map.Layers, b.Map.ToLat, b.Map.ToLon, b.Map.Animate = []MapLayer{FlowLayer}, "to_lat", "to_lon", true
		},
	} {
		b := drops()
		edit(&b)
		if err := mapReport(b).Validate(); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
}

// Each is something drawn as nothing, and refused with a sentence rather than
// quietly ignored.
func TestAMapIsRefusedMarksItCannotDraw(t *testing.T) {
	for name, c := range map[string]struct {
		edit func(*Block)
		says string
	}{
		"a radius of nothing": {func(b *Block) { b.Map.Layers = []MapLayer{RadiusLayer} }, "needs radiusKm"},
		"a radius past a continent": {func(b *Block) {
			b.Map.Layers, b.Map.RadiusKm = []MapLayer{RadiusLayer}, MaxRadiusKm+1
		}, "needs radiusKm"},
		"a radius with no circles": {func(b *Block) { b.Map.RadiusKm = 10 }, "draws no radius layer"},
		"still flows":              {func(b *Block) { b.Map.Animate = true }, "draws none"},
		"names for hexagons": {func(b *Block) {
			b.Map.Layers, b.Map.Labels = []MapLayer{HexbinLayer}, true
		}, "no region or place"},
	} {
		b := drops()
		c.edit(&b)
		err := mapReport(b).Validate()
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want it to mention %q", name, err, c.says)
		}
	}
}
