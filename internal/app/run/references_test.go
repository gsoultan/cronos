package run_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/document"
)

/*
A reference line is what a business chart is read against: a target, an
SLA, the average. Drawn as a series of one repeated number it read as data
and sat in the legend; drawn off the top of a scale fitted to the marks, a
target nobody reached was not on the chart at all.
*/

// Months are €982,571 and €1,337,000: a €2M target is above both, and the
// scale reaches it.
func TestATargetAboveTheMarksIsOnTheScale(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: column
  x: {field: month}
  y: {field: revenue}
  references:
    - {value: 2000000, label: Target}
    - {stat: avg}`)[0]
	if len(b.References) != 2 {
		t.Fatalf("%d lines, want 2: %+v", len(b.References), b.References)
	}
	target, avg := b.References[0], b.References[1]
	if target.Label != "Target" || target.Formatted != "€2M" {
		t.Errorf("the target reads %q %q, want Target €2M", target.Label, target.Formatted)
	}
	if b.YAxis.Max < 2000000 {
		t.Errorf("the scale stops at %v, under the target", b.YAxis.Max)
	}
	if top := b.YAxis.Ticks[len(b.YAxis.Ticks)-1].Label; !strings.HasPrefix(top, "€") {
		t.Errorf("the widened scale lost its currency: %q", top)
	}
	if avg.Label != "Average" || avg.Value < 1159785 || avg.Value > 1159786 {
		t.Errorf("the average is %q at %v, want Average at 1,159,785.44", avg.Label, avg.Value)
	}
	if avg.Formatted != "€1.2M" {
		t.Errorf("the average reads %q, want €1.2M", avg.Formatted)
	}
}

// A stacked chart's stat is of its stacks, which is what the eye compares.
func TestAStackedChartsAverageIsOfItsStacks(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: column
  x: {field: month}
  y: {field: units}
  series: {field: region}
  stacked: true
  references: [{stat: max}]`)[0]
	if len(b.References) != 1 || b.References[0].Value != 9 {
		t.Errorf("the highest stack is %+v, want 9 (Feb: 4 + 5)", b.References)
	}
}

// An average of counts reads as a count.
func TestAnAverageOfCountsIsWhole(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: column
  x: {field: month}
  series: {field: region}
  y: {field: units}
  references: [{stat: avg}]`)[0]
	// Side by side, the bars are 3, 4, 2 and 5: a mean of 3.5.
	if r := b.References[0]; r.Value != 3.5 || r.Formatted != "4" {
		t.Errorf("the average is %v and reads %q, want 3.5 written 4", r.Value, r.Formatted)
	}
}

// On paper the line is a dashed rule across the box, named at its end.
func TestAReferencePrintsAsADashedRule(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: line
  x: {field: month}
  y: {field: revenue}
  references: [{value: 2000000, label: Target}]`)[0]
	c := run.Printable(run.View{Blocks: []run.Block{b}})[0]
	var dashed []document.Mark
	for _, m := range marksOf(c, document.LineMark) {
		if m.Dash {
			dashed = append(dashed, m)
		}
	}
	if len(dashed) != 1 || dashed[0].Points[0][0] != 0 || dashed[0].Points[1][0] != 1 {
		t.Fatalf("dashed rules %+v, want one across the box", dashed)
	}
	if !strings.Contains(words(c), "Target €2M") {
		t.Errorf("the rule is not named: %q", words(c))
	}
}

// The lines travel to the browser, and a chart without any says nothing.
func TestReferencesAreSentOnlyWhereDrawn(t *testing.T) {
	b := drawFormatted(t, `- kind: chart
  chart: column
  x: {field: region}
  y: {field: revenue}
  references: [{value: 2000000, label: Target}]
- kind: chart
  chart: column
  x: {field: region}
  y: {field: revenue}`)
	with, _ := json.Marshal(b[0])
	without, _ := json.Marshal(b[1])
	if !strings.Contains(string(with), `"references":[{"label":"Target","value":2000000,"formatted":"€2M"}]`) {
		t.Errorf("the line was not sent: %s", with)
	}
	if strings.Contains(string(without), "references") {
		t.Errorf("a chart with no lines sent some: %s", without)
	}
}
