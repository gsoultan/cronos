package run_test

import (
	"testing"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/document"
)

/*
A Pareto chart is the few that make most of the whole: categories largest
first as columns, and the running share of the whole as a line on a scale
of its own to 100%. Without it, the nearest a report came was a bar chart
sorted by hand and a reader adding up the bars.
*/

// North is €2.2M of €2.3M: the line is 96% after it and all of it after South.
func TestAParetoRanksAndRunsUpToTheWhole(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: pareto
  x: {field: region}
  y: {field: revenue}
  sort: [{field: region, dir: desc}]`)[0]
	if len(b.Tracks) != 2 {
		t.Fatalf("%d tracks, want columns and a line", len(b.Tracks))
	}
	cols, line := b.Tracks[0], b.Tracks[1]
	if cols.Draw != "bar" || line.Draw != "line" || !line.Secondary {
		t.Errorf("drawn as %s and %s (secondary %v)", cols.Draw, line.Draw, line.Secondary)
	}
	// Largest first, even against the author's sort.
	if cols.Bars[0].Label != "North" || cols.Bars[0].Formatted != "€2.2M" {
		t.Errorf("first column %+v, want North at €2.2M", cols.Bars[0])
	}
	if cols.Label != "revenue" {
		t.Errorf("the columns are called %q", cols.Label)
	}
	if got := line.Bars[0].Formatted; got != "96.2%" {
		t.Errorf("the share after North reads %q, want 96.2%%", got)
	}
	if last := line.Bars[len(line.Bars)-1]; last.Value != 1 || last.Formatted != "100%" {
		t.Errorf("the line ends at %+v, want all of it", last)
	}
	if b.Axis2 == nil || b.Axis2.Min != 0 || b.Axis2.Max != 1 || b.Axis2.Ticks[4].Label != "100%" {
		t.Errorf("the share scale is %+v, want 0%%–100%%", b.Axis2)
	}
	if b.YAxis.Ticks[0].Label != "€0" {
		t.Errorf("the column scale is not in euros: %+v", b.YAxis.Ticks)
	}
}

// A count's columns are called for what was counted.
func TestAParetoOfACountIsCalledForItsRows(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: pareto
  x: {field: region}
  y: {field: units, aggregate: count}`)[0]
	if b.Tracks[0].Label != "Count" {
		t.Errorf("the columns are called %q, want Count", b.Tracks[0].Label)
	}
}

// On paper it is a combo: columns, and a line against the right scale.
func TestAParetoPrintsAsColumnsAndALine(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: pareto
  x: {field: region}
  y: {field: revenue}`)[0]
	c := run.Printable(run.View{Blocks: []run.Block{b}})[0]
	if len(marksOf(c, document.RectMark)) != 2 || len(marksOf(c, document.LineMark)) != 1 {
		t.Errorf("%d columns and %d lines, want 2 and 1",
			len(marksOf(c, document.RectMark)), len(marksOf(c, document.LineMark)))
	}
	if len(c.Ticks2) == 0 || c.Ticks2[len(c.Ticks2)-1].Label != "100%" {
		t.Errorf("the right scale is %+v", c.Ticks2)
	}
}
