package run_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/document"
)

/*
Calendars and trended stats over the depots' days: London and Edinburgh
delivered on the first of August, 1,900 parcels between them, and Bristol
its 300 on the second.
*/

// A calendar's cells are days, dated as ISO 8601 whatever the database
// returned them as, each shaded — whether or not its x said day.
func TestACalendarDrawsDays(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: calendar
  title: By day
  x: {field: day}
  y: {field: parcels, aggregate: sum}`)
	if len(b.Days) != 2 || b.Days[0].Date != "2026-08-01" || b.Days[0].Value != 1900 || b.Days[1].Date != "2026-08-02" {
		t.Fatalf("days %+v", b.Days)
	}
	if b.Days[0].Step <= b.Days[1].Step {
		t.Errorf("the busier day is not the darker: %+v", b.Days)
	}
	if out := marshal(t, b); out["days"] == nil {
		t.Errorf("the payload has no days: %v", keys(out))
	}
	c := run.Printable(run.View{Blocks: []run.Block{b}})[0]
	if len(marksOf(c, document.RectMark)) != 365 || len(c.Keys) != 2 || !strings.Contains(words(c), "2026") {
		t.Errorf("%d days printed, keys %+v, words %q", len(marksOf(c, document.RectMark)), c.Keys, words(c))
	}
}

// A stat with a trend draws its number over the periods under it, and says
// how the last moved against the one before — good news or not by what the
// definition says the measure is.
func TestAStatTrendsAndSaysHowItMoved(t *testing.T) {
	b := draw(t, `- kind: stat
  label: Parcels
  value: {field: parcels, aggregate: sum}
  trend: {field: day, grain: day}`)
	if b.Value != "2,200" || len(b.Trend) != 2 || b.Trend[1].Value != 300 {
		t.Fatalf("value %q, trend %+v", b.Value, b.Trend)
	}
	if d := b.Delta; d == nil || d.Value != "-84.2%" || d.Dir != "down" || d.Good || !strings.HasPrefix(d.Label, "vs ") {
		t.Errorf("delta %+v, want -84.2%% down and bad news", b.Delta)
	}
	out := marshal(t, b)
	if out["trend"] == nil || out["delta"] == nil {
		t.Errorf("the payload has no trend or delta: %v", keys(out))
	}
	lower := draw(t, `- kind: stat
  label: Parcels
  value: {field: parcels, aggregate: sum}
  trend: {field: day, grain: day, better: lower}`)
	if lower.Delta == nil || !lower.Delta.Good {
		t.Errorf("a fall where lower is better is not good news: %+v", lower.Delta)
	}
}

func TestWhatTheTimeChartsRefuse(t *testing.T) {
	for _, c := range []struct{ name, block, says string }{
		{"a calendar draws days", `- kind: chart
  chart: calendar
  title: t
  x: {field: day, grain: month}
  y: {field: parcels}`, "draws days"},
		{"only a stat trends", `- kind: chart
  chart: bar
  title: t
  x: {field: region}
  y: {field: parcels}
  trend: {field: day, grain: day}`, "only a stat"},
		{"a trend's periods are ones a date has", `- kind: stat
  label: t
  value: {field: parcels}
  trend: {field: day, grain: fortnight}`, "want day, week"},
		{"better is higher or lower", `- kind: stat
  label: t
  value: {field: parcels}
  trend: {field: day, grain: day, better: sideways}`, "higher or lower"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := load(c.block)
			if !errors.Is(err, definition.ErrInvalid) || !strings.Contains(err.Error(), c.says) {
				t.Errorf("err = %v, want it refused saying %q", err, c.says)
			}
		})
	}
}
