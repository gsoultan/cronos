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
Column, radar and bullet charts, and stacks drawn to their whole — what each
carries to a browser and to paper, and what each refuses when it is written.
*/

// A column chart is read against a scale, as a line is, and it stands on
// nothing: a bar's figure sits beside it, a column's height is read off the
// scale down its left.
func TestAColumnChartCarriesAScaleFromNothing(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: column
  title: By region
  x: {field: region}
  y: {field: parcels, aggregate: sum}`)
	if b.YAxis == nil || b.YAxis.Min != 0 || b.YAxis.Max < 1500 {
		t.Fatalf("scale %+v, want nothing to past 1,500", b.YAxis)
	}
	out := marshal(t, b)
	for _, k := range []string{"series", "yAxis"} {
		if _, ok := out[k]; !ok {
			t.Errorf("%q is missing from a column chart's payload: %v", k, keys(out))
		}
	}
}

// Stacked, the scale reaches the stack's height, not its tallest part.
func TestAStackedColumnScalesToItsStacks(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: column
  title: By carrier
  stacked: true
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)
	if b.YAxis == nil || b.YAxis.Max < 1500 || !b.Stacked || len(b.Totals) != 2 {
		t.Fatalf("scale %+v, stacked %v, totals %+v", b.YAxis, b.Stacked, b.Totals)
	}
}

// Stacked to a whole, each part is its share of its bucket: the payload says
// so and carries a scale of percentages, and the values stay the values.
func TestAStackOfSharesIsReadAgainstPercentages(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: column
  title: Share by carrier
  stacked: percent
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)
	if !b.Percent || !b.Stacked {
		t.Fatalf("percent %v, stacked %v", b.Percent, b.Stacked)
	}
	ticks := b.YAxis.Ticks
	if b.YAxis.Min != 0 || b.YAxis.Max != 1 || ticks[len(ticks)-1].Label != "100%" {
		t.Errorf("scale %+v, want 0 to 100%%", b.YAxis)
	}
	if b.Groups[0].Bars[0].Value != 1200 {
		t.Errorf("a share replaced its value: %+v", b.Groups[0].Bars[0])
	}
	if out := marshal(t, b); out["percent"] != true {
		t.Errorf("the payload does not say percent: %v", keys(out))
	}
}

// A radar's middle is nothing, and each series is a shape through a value per
// spoke.
func TestARadarIsReadFromItsMiddle(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: radar
  title: Profile
  x: {field: depot}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)
	if b.YAxis == nil || b.YAxis.Min != 0 || len(b.Groups) != 2 || len(b.Groups[0].Bars) != 3 {
		t.Fatalf("scale %+v, %d series of %d spokes", b.YAxis, len(b.Groups), len(b.Groups[0].Bars))
	}
}

// Each bullet is a category's value against its target, all read along one
// scale that reaches the furthest of either.
func TestABulletReadsEachCategoryAgainstItsTarget(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: bullet
  title: Against plan
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  target: {value: 1000, label: Plan}`)
	if len(b.Bullets) != 2 {
		t.Fatalf("%d bullets, want one a region", len(b.Bullets))
	}
	england := b.Bullets[0]
	if england.Label != "England" || england.Value != 1500 || england.Target != 1000 || england.TargetFormatted != "1,000" {
		t.Errorf("England reads %+v", england)
	}
	if b.XAxis == nil || b.XAxis.Min != 0 || b.XAxis.Max < 1500 {
		t.Errorf("scale %+v, want nothing to past 1,500", b.XAxis)
	}
	if len(b.Bands) != 2 {
		t.Errorf("bands %v, want the two defaults", b.Bands)
	}
	out := marshal(t, b)
	for _, k := range []string{"bullets", "xAxis", "bands", "series"} {
		if _, ok := out[k]; !ok {
			t.Errorf("%q is missing from a bullet chart's payload: %v", k, keys(out))
		}
	}
}

// A bullet with no x is the whole set against its target, once — here a
// target that is a column, read beside the value.
func TestABulletWithoutCategoriesIsOneRow(t *testing.T) {
	b := draw(t, `- kind: chart
  chart: bullet
  title: Against staff
  y: {field: parcels, aggregate: sum}
  target: {field: staff, aggregate: sum}
  bands: [0.5, 0.75]`)
	if len(b.Bullets) != 1 || b.Bullets[0].Label != "" || b.Bullets[0].Value != 2200 || b.Bullets[0].Target != 77 {
		t.Fatalf("bullets %+v", b.Bullets)
	}
	if len(b.Bands) != 2 || b.Bands[0] != 0.5 {
		t.Errorf("bands %v, want the report's own", b.Bands)
	}
}

func TestWhatTheNewChartsRefuse(t *testing.T) {
	for _, c := range []struct{ name, block, says string }{
		{"a radar cannot stack", `- kind: chart
  chart: radar
  title: t
  stacked: true
  x: {field: depot}
  series: {field: carrier}
  y: {field: parcels}`, "stacked"},
		{"a line cannot stack to a whole", `- kind: chart
  chart: line
  title: t
  stacked: percent
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels}`, "stacked"},
		{"an area does not stack to a whole yet", `- kind: chart
  chart: area
  title: t
  stacked: percent
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels}`, "area chart does not"},
		{"a bullet needs a target", `- kind: chart
  chart: bullet
  title: t
  x: {field: region}
  y: {field: parcels}`, "no target"},
		{"only a bullet has bands", `- kind: chart
  chart: bar
  title: t
  bands: [0.5]
  x: {field: region}
  y: {field: parcels}`, "bands"},
		{"bands run upwards", `- kind: chart
  chart: bullet
  title: t
  bands: [0.9, 0.6]
  y: {field: parcels}
  target: {value: 10}`, "ascending"},
		{"a column has no target", `- kind: chart
  chart: column
  title: t
  x: {field: region}
  y: {field: parcels}
  target: {value: 10}`, "target"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := load(c.block)
			if !errors.Is(err, definition.ErrInvalid) || !strings.Contains(err.Error(), c.says) {
				t.Errorf("err = %v, want it refused saying %q", err, c.says)
			}
		})
	}
}

// On paper as on screen: a column chart stands its columns up over its
// categories, a radar is a square of rings and shapes, and a bullet chart is
// a row of shades, a bar and a target mark per category.
func TestTheNewChartsPrint(t *testing.T) {
	col := run.Printable(run.View{Blocks: []run.Block{draw(t, `- kind: chart
  chart: column
  title: By region
  x: {field: region}
  y: {field: parcels, aggregate: sum}`)}})[0]
	bars := marksOf(col, document.RectMark)
	if len(bars) != 2 || bars[0].H <= bars[0].W || len(col.XTicks) != 2 || len(col.Ticks) == 0 {
		t.Errorf("columns %+v under %+v", bars, col.XTicks)
	}
	radar := run.Printable(run.View{Blocks: []run.Block{draw(t, `- kind: chart
  chart: radar
  title: Profile
  x: {field: depot}
  y: {field: parcels, aggregate: sum}`)}})[0]
	if radar.Aspect != 1 || len(marksOf(radar, document.PolyMark)) != 1 || !strings.Contains(words(radar), "Edinburgh") {
		t.Errorf("radar aspect %v, %d shapes, words %q", radar.Aspect, len(marksOf(radar, document.PolyMark)), words(radar))
	}
	bullet := run.Printable(run.View{Blocks: []run.Block{draw(t, `- kind: chart
  chart: bullet
  title: Against plan
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  target: {value: 1000}`)}})[0]
	if bullet.Height <= 0 || !strings.Contains(words(bullet), "1,500 of 1,000 · 150%") || len(bullet.XTicks) == 0 {
		t.Errorf("bullet %vmm tall saying %q along %+v", bullet.Height, words(bullet), bullet.XTicks)
	}
}

// A bar chart stacked to its whole prints each part as its share of its
// bucket: England's Aurora is four fifths of its bar, Baltic the rest.
func TestABarStackOfSharesPrintsShares(t *testing.T) {
	c := run.Printable(run.View{Blocks: []run.Block{draw(t, `- kind: chart
  chart: bar
  title: Share by carrier
  stacked: percent
  x: {field: region}
  series: {field: carrier}
  y: {field: parcels, aggregate: sum}`)}})[0]
	parts := marksOf(c, document.RectMark)
	if len(parts) != 3 {
		t.Fatalf("%d parts, want England's two and Scotland's one", len(parts))
	}
	if ratio := parts[0].W / parts[1].W; ratio < 3.9 || ratio > 4.1 {
		t.Errorf("Aurora is %.2f times Baltic's length, want 4 — 1,200 and 300 of 1,500", ratio)
	}
	if england, scotland := parts[0].W+parts[1].W, parts[2].W; england < scotland*0.98 || england > scotland*1.02 {
		t.Errorf("England's whole is %.3f and Scotland's %.3f, want both the full length", england, scotland)
	}
}
