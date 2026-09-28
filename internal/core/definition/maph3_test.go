package definition

import (
	"errors"
	"strings"
	"testing"
)

func TestAMapShadesH3Cells(t *testing.T) {
	for name, edit := range map[string]func(*Block){
		"binned from places": func(b *Block) { b.Map.Layers = []MapLayer{H3Layer} },
		"at a resolution":    func(b *Block) { b.Map.Layers, b.Map.H3Resolution = []MapLayer{H3Layer}, 7 },
		"indexed cells": func(b *Block) {
			b.X.Field = "cell"
			b.Map = &MapSpec{Layers: []MapLayer{H3Layer}, H3: "cell"}
		},
		// A row per cell: nothing is added up twice, so an average stands.
		"averaged, a row per cell": func(b *Block) {
			b.X.Field, b.Y.Aggregate = "cell", "avg"
			b.Map = &MapSpec{Layers: []MapLayer{H3Layer}, H3: "cell"}
		},
		"with dots over them": func(b *Block) { b.Map.Layers = []MapLayer{H3Layer, ScatterLayer} },
	} {
		b := drops()
		edit(&b)
		if err := mapReport(b).Validate(); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
}

func TestAnH3MapIsRefusedWhatItCannotDraw(t *testing.T) {
	for name, c := range map[string]struct {
		edit func(*Block)
		says string
	}{
		"no cells and no places": {func(b *Block) {
			b.Map = &MapSpec{Layers: []MapLayer{H3Layer}}
		}, "reads its cells from map.h3"},
		"beside shaded regions": {func(b *Block) {
			b.Map.Layers, b.Map.Geometry = []MapLayer{H3Layer, PolygonLayer}, "zone"
		}, "one legend cannot explain both"},
		"beside hexagons":     {func(b *Block) { b.Map.Layers = []MapLayer{H3Layer, HexbinLayer} }, "one legend"},
		"finer than H3 has":   {func(b *Block) { b.Map.Layers, b.Map.H3Resolution = []MapLayer{H3Layer}, 16 }, "outside 1–15"},
		"cells with no layer": {func(b *Block) { b.Map.H3 = "cell" }, "draws no h3 layer"},
		// Places binned into a cell are added up: an average of averages.
		"averaged places": {func(b *Block) {
			b.Map.Layers, b.Y.Aggregate = []MapLayer{H3Layer}, "avg"
		}, "average"},
		"averaged, rolled up": {func(b *Block) {
			b.X.Field, b.Y.Aggregate = "cell", "avg"
			b.Map = &MapSpec{Layers: []MapLayer{H3Layer}, H3: "cell", H3Resolution: 5}
		}, "average"},
	} {
		b := drops()
		c.edit(&b)
		err := mapReport(b).Validate()
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want it to mention %q", name, err, c.says)
		}
	}
}
