package definition

import (
	"errors"
	"strings"
	"testing"
)

func TestAMapPlaysThroughTime(t *testing.T) {
	b := drops()
	b.Map.Time = &DimensionRef{Field: "day", Grain: "week"}
	if err := mapReport(b).Validate(); err != nil {
		t.Errorf("refused: %v", err)
	}
	if !b.Folds() {
		t.Error("a timed map adds each place up across its periods, and says it folds")
	}
}

func TestATimedMapIsRefusedWhatItCannotPlay(t *testing.T) {
	for name, c := range map[string]struct {
		edit func(*Block)
		says string
	}{
		"no field":      {func(b *Block) { b.Map.Time = &DimensionRef{Grain: "day"} }, "names no field"},
		"no such grain": {func(b *Block) { b.Map.Time = &DimensionRef{Field: "day", Grain: "hour"} }, `grain "hour"`},
		"hexagons": {func(b *Block) {
			b.Map.Layers, b.Map.Time = []MapLayer{HexbinLayer}, &DimensionRef{Field: "day", Grain: "day"}
		}, "give them a block of their own"},
		// Each place is added up across periods for the map as it opens.
		"averaged": {func(b *Block) {
			b.Y.Aggregate, b.Map.Time = "avg", &DimensionRef{Field: "day", Grain: "day"}
		}, "average"},
		// And sized by one: a place's size adds up across periods too.
		"sized by an average": {func(b *Block) {
			b.Size, b.Map.Time = MeasureRef{Field: "parcels", Aggregate: "avg"}, &DimensionRef{Field: "day", Grain: "day"}
		}, "average"},
		"an overlay that plays": {func(b *Block) {
			ov := drops()
			ov.Kind, ov.Chart = "", ""
			ov.Map.Time = &DimensionRef{Field: "day", Grain: "day"}
			b.Map.Overlays = []Block{ov}
		}, "an overlay is drawn over the map as it is"},
	} {
		b := drops()
		c.edit(&b)
		err := mapReport(b).Validate()
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want it to mention %q", name, err, c.says)
		}
	}
}
