package definition

import (
	"errors"
	"strings"
	"testing"
)

func TestDataBarsAndSparklinesAreChecked(t *testing.T) {
	table := Block{Kind: TableBlock, Columns: []string{"customer", "total"}, Bars: []string{"total"}}
	if err := table.validateShape("interactive", 0); err != nil {
		t.Fatalf("a table with bars in one of its columns was refused: %v", err)
	}
	spark := Block{Kind: PivotBlock, Value: MeasureRef{Field: "amount", Aggregate: "sum"},
		Pivot: &Pivot{Rows: DimensionRef{Field: "line"},
			Columns: DimensionRef{Field: "booked_on", Grain: "month"}, Sparkline: true}}
	if err := spark.validateShape("interactive", 0); err != nil {
		t.Fatalf("a pivot with a sparkline across months was refused: %v", err)
	}

	for _, c := range []struct {
		name string
		b    Block
		says string
	}{
		{"a column it does not list", Block{Kind: TableBlock, Columns: []string{"customer"},
			Bars: []string{"total"}}, "not one of its columns"},
		{"twice", Block{Kind: TableBlock, Columns: []string{"total"},
			Bars: []string{"total", "total"}}, "twice"},
		{"on a chart", Block{Kind: ChartBlock, Chart: BarChart, X: DimensionRef{Field: "a"},
			Y: MeasureRef{Field: "b"}, Bars: []string{"b"}}, "only a table"},
		{"a sparkline across categories", Block{Kind: PivotBlock, Value: MeasureRef{Field: "amount"},
			Pivot: &Pivot{Rows: DimensionRef{Field: "line"}, Columns: DimensionRef{Field: "region"},
				Sparkline: true}}, "not a date"},
	} {
		err := c.b.validateShape("interactive", 0)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want it to say %q", c.name, err, c.says)
		}
	}
}
