package definition

import (
	"errors"
	"strings"
	"testing"
)

func refOf(v float64) *float64 { return &v }

// A reference line is checked where it is declared: on a chart with a value
// scale, at a value or a stat and not both, and no more than three.
func TestReferencesAreChecked(t *testing.T) {
	column := func(refs ...Reference) Block {
		return Block{Kind: ChartBlock, Chart: ColumnChart, References: refs}
	}
	for _, c := range []struct {
		name string
		b    Block
		says string
	}{
		{"a pie", Block{Kind: ChartBlock, Chart: PieChart, References: []Reference{{Value: refOf(1)}}}, "value scale"},
		{"shares", Block{Kind: ChartBlock, Chart: ColumnChart, Stacked: StackedShares,
			References: []Reference{{Value: refOf(1)}}}, "stacked to its whole"},
		{"neither", column(Reference{Label: "Target"}), "a value or a stat"},
		{"both", column(Reference{Value: refOf(1), Stat: "avg"}), "a value or a stat"},
		{"a stat nobody works out", column(Reference{Stat: "mode"}), "avg, median, min or max"},
		{"four", column(Reference{Stat: "avg"}, Reference{Stat: "min"}, Reference{Stat: "max"},
			Reference{Stat: "median"}), "at most 3"},
	} {
		err := c.b.validateReferences("interactive", 0)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v, want it to say %q", c.name, err, c.says)
		}
	}
	ok := column(Reference{Value: refOf(500000), Label: "Target"}, Reference{Stat: "avg"})
	if err := ok.validateReferences("interactive", 0); err != nil {
		t.Errorf("a target and an average were refused: %v", err)
	}
}

func TestAStatsLineIsNamedForTheStat(t *testing.T) {
	for stat, want := range map[string]string{"avg": "Average", "median": "Median", "min": "Lowest", "max": "Highest"} {
		if got := (Reference{Stat: stat}).Heading(); got != want {
			t.Errorf("%s is called %q, want %q", stat, got, want)
		}
	}
	if got := (Reference{Stat: "avg", Label: "Mean"}).Heading(); got != "Mean" {
		t.Errorf("a label was overridden: %q", got)
	}
}
