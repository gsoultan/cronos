package definition

import (
	"errors"
	"strings"
	"testing"
)

func TestAFacetIsChecked(t *testing.T) {
	chart := func(edit func(*Block)) Block {
		b := Block{Kind: ChartBlock, Chart: LineChart, Facet: true,
			X: DimensionRef{Field: "day", Grain: "month"}, Y: MeasureRef{Field: "revenue"},
			Series: DimensionRef{Field: "region"}}
		edit(&b)
		return b
	}
	if err := chart(func(*Block) {}).validateChart("interactive", 0); err != nil {
		t.Fatalf("a faceted line chart was refused: %v", err)
	}
	for _, c := range []struct {
		name string
		b    Block
		says string
	}{
		{"a pie", chart(func(b *Block) { b.Chart = PieChart }), "a pie chart cannot"},
		{"no series", chart(func(b *Block) { b.Series = DimensionRef{} }), "faceted by nothing"},
		{"stacked", chart(func(b *Block) { b.Chart = ColumnChart; b.Stacked = "true" }), "stacked and faceted"},
	} {
		err := c.b.validateChart("interactive", 0)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want it to say %q", c.name, err, c.says)
		}
	}
}
