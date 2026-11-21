package definition

import (
	"errors"
	"strings"
	"testing"
)

func TestATimelineIsChecked(t *testing.T) {
	plan := func(edit func(*Block)) Block {
		b := Block{Kind: ChartBlock, Chart: TimelineChart, X: DimensionRef{Field: "project"},
			Start: DimensionRef{Field: "starts"}, End: DimensionRef{Field: "ends"}}
		edit(&b)
		return b
	}
	if err := plan(func(*Block) {}).validateChart("interactive", 0); err != nil {
		t.Fatalf("a plain timeline was refused: %v", err)
	}
	if err := plan(func(b *Block) { b.Today = true }).validateChart("interactive", 0); err != nil {
		t.Fatalf("a timeline with today was refused: %v", err)
	}
	for _, c := range []struct {
		name string
		b    Block
		says string
	}{
		{"no x", plan(func(b *Block) { b.X = DimensionRef{} }), "what each bar is"},
		{"no end", plan(func(b *Block) { b.End = DimensionRef{} }), "a start and an end"},
		{"a measure", plan(func(b *Block) { b.Y = MeasureRef{Field: "cost"} }), "as long as its dates"},
		{"a grain", plan(func(b *Block) { b.Start.Grain = "month" }), "day to day"},
		{"dates on a column chart", Block{Kind: ChartBlock, Chart: ColumnChart,
			X: DimensionRef{Field: "a"}, Y: MeasureRef{Field: "b"}, Start: DimensionRef{Field: "s"}},
			"only a timeline"},
		{"today on a column chart", Block{Kind: ChartBlock, Chart: ColumnChart,
			X: DimensionRef{Field: "a"}, Y: MeasureRef{Field: "b"}, Today: true},
			"only a timeline does"},
	} {
		err := c.b.validateChart("interactive", 0)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want it to say %q", c.name, err, c.says)
		}
	}
}
