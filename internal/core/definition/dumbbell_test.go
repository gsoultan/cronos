package definition

import (
	"errors"
	"strings"
	"testing"
)

func TestADumbbellIsChecked(t *testing.T) {
	two := []Metric{{Field: "sla", Aggregate: "avg"}, {Field: "took", Aggregate: "avg"}}
	bell := func(edit func(*Block)) Block {
		b := Block{Kind: ChartBlock, Chart: DumbbellChart, X: DimensionRef{Field: "team"}, Metrics: two}
		edit(&b)
		return b
	}
	if err := bell(func(*Block) {}).validateChart("interactive", 0); err != nil {
		t.Fatalf("a plain dumbbell was refused: %v", err)
	}
	for _, c := range []struct {
		name string
		b    Block
		says string
	}{
		{"one metric", bell(func(b *Block) { b.Metrics = two[:1] }), "want two"},
		{"no metrics", bell(func(b *Block) { b.Metrics = nil; b.Y = MeasureRef{Field: "took"} }), "sets y"},
		{"no x", bell(func(b *Block) { b.X = DimensionRef{} }), "what each row is"},
		{"a series", bell(func(b *Block) { b.Series = DimensionRef{Field: "region"} }), "splits by series"},
	} {
		err := c.b.validateChart("interactive", 0)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want it to say %q", c.name, err, c.says)
		}
	}
}
