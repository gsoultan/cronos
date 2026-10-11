package definition

import (
	"errors"
	"strings"
	"testing"
)

func crossed() Block {
	return Block{
		Kind: PivotBlock, Title: "Revenue by region and month",
		Value: MeasureRef{Field: "total"},
		Pivot: &Pivot{
			Rows:    DimensionRef{Field: "region"},
			Columns: DimensionRef{Field: "issued_at", Grain: "month"},
		},
	}
}

func TestAPivotValidates(t *testing.T) {
	if err := mapReport(crossed()).Validate(); err != nil {
		t.Fatalf("refused: %v", err)
	}
}

func TestAPivotIsRefused(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Block)
		says string
	}{
		{"no pivot", func(b *Block) { b.Pivot = nil }, "needs rows and columns to cross"},
		{"no columns", func(b *Block) { b.Pivot.Columns = DimensionRef{} }, "needs both rows and columns"},
		{"crossed with itself", func(b *Block) { b.Pivot.Columns = b.Pivot.Rows }, "with itself"},
		{"no measure", func(b *Block) { b.Value = MeasureRef{} }, "set value"},
		{"a grain that is not one", func(b *Block) { b.Pivot.Columns.Grain = "fortnight" }, `"fortnight"`},
		{"a pivot on a table", func(b *Block) { b.Kind = TableBlock; b.Columns = []string{"region"} }, "set kind: pivot"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := crossed()
			p := *b.Pivot
			b.Pivot = &p
			c.edit(&b)
			err := mapReport(b).Validate()
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.says) {
				t.Errorf("err = %v, want one saying %q", err, c.says)
			}
		})
	}
}

func TestAPivotSheetIsOneThingOrTheOther(t *testing.T) {
	tab := func(s Sheet) Report {
		return Report{Name: "x", Dataset: "d",
			Outputs: []Output{{Name: "xlsx", Renderer: Spreadsheet, Sheets: []Sheet{s}}}}
	}
	p := crossed()
	if err := tab(Sheet{Name: "A", Value: p.Value, Pivot: p.Pivot}).Validate(); err != nil {
		t.Errorf("a pivot tab was refused: %v", err)
	}
	for says, s := range map[string]Sheet{
		"one or the other":   {Name: "A", Columns: []string{"id"}, Value: p.Value, Pivot: p.Pivot},
		"only a pivot reads": {Name: "A", Columns: []string{"id"}, Value: p.Value},
		"needs both rows":    {Name: "A", Value: p.Value, Pivot: &Pivot{Rows: p.Pivot.Rows}},
	} {
		if err := tab(s).Validate(); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), says) {
			t.Errorf("err = %v, want one saying %q", err, says)
		}
	}
}
