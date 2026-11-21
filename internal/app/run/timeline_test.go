package run_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/document"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
A timeline is each thing as a bar from when it starts to when it ends: a
roadmap, a project plan. Its phases are rows, and the bar runs from the first
to start to the last to finish.
*/

const timelineSchema = `
CREATE TABLE phases (project TEXT, status TEXT, starts TEXT, ends TEXT);
INSERT INTO phases VALUES
  ('Warehouse', 'Done',    '2026-01-05', '2026-02-27'),
  ('Warehouse', 'Done',    '2026-03-02', '2026-03-27'),
  ('Portal',    'At risk', '2026-02-16', '2026-07-31'),
  ('Launch',    'Done',    '2026-01-12', '2026-01-12'),
  ('Undated',   'Planned', NULL,         '2026-05-01');`

const timelineDataset = `
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: phases}
spec:
  sources: [{ref: warehouse}]
  query: SELECT project, status, starts, ends FROM phases
  fields:
    - {name: project, type: string, role: dimension}
    - {name: status,  type: string, role: dimension}
    - {name: starts,  type: date,   role: dimension}
    - {name: ends,    type: date,   role: dimension}
`

func drawTimeline(t *testing.T) run.Block { return drawPlan(t, "", time.Now) }

// drawPlan draws the plan with extra lines on its chart, on a clock.
func drawPlan(t *testing.T, extra string, now func() time.Time) run.Block {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+strings.ReplaceAll(t.Name(), "/", "-")+fmt.Sprint(time.Now().UnixNano())+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(timelineSchema); err != nil {
		t.Fatal(err)
	}
	ds, err := yamlcodec.Loader{}.Dataset([]byte(timelineDataset))
	if err != nil {
		t.Fatalf("dataset: %v", err)
	}
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: plan}
spec:
  dataset: phases
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: chart
          chart: timeline
          title: Plan
          x: {field: project}
          start: {field: starts}
          end: {field: ends}
          series: {field: status}
` + extra))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	s := run.New(datasets{"phases": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
	}}).WithClock(now)
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return v.Blocks[0]
}

func TestATimelineRunsFromTheFirstPhaseToTheLast(t *testing.T) {
	b := drawTimeline(t)
	got := []string{}
	for _, s := range b.Spans {
		got = append(got, s.Label)
	}
	// In order of starting; a row with no start is no bar.
	if strings.Join(got, ",") != "Warehouse,Launch,Portal" {
		t.Fatalf("bars %v, want Warehouse, Launch, Portal", got)
	}
	w := b.Spans[0]
	if w.Start != "5 Jan 2026" || w.End != "27 Mar 2026" || w.Said != "5 Jan – 27 Mar 2026 · 12 weeks" {
		t.Errorf("the warehouse reads %q to %q, %q", w.Start, w.End, w.Said)
	}
	if launch := b.Spans[1]; launch.To-launch.From != 1 || launch.Said != "12 Jan 2026" {
		t.Errorf("a one-day phase is %v days, %q", launch.To-launch.From, launch.Said)
	}
	if b.Spans[0].Slot != 0 || b.Spans[2].Slot != 1 || b.Spans[2].Series != "At risk" {
		t.Errorf("coloured %+v", b.Spans)
	}
	a := b.XAxis
	if a == nil || a.Ticks[0].Label != "Jan 2026" || a.Ticks[1].Label != "Feb" {
		t.Fatalf("the calendar is %+v", a)
	}
	if a.Min > b.Spans[0].From || a.Max < b.Spans[2].To {
		t.Errorf("the calendar %v–%v does not reach every bar", a.Min, a.Max)
	}
}

func TestATimelinePrintsABarAndANameARow(t *testing.T) {
	c := run.Printable(run.View{Blocks: []run.Block{drawTimeline(t)}})[0]
	bars := marksOf(c, document.RectMark)
	// The one-day launch is a milestone, a point rather than a bar.
	if len(bars) != 2 || bars[0].Value != "5 Jan – 27 Mar 2026 · 12 weeks" {
		t.Errorf("%d bars, the first %+v", len(bars), bars)
	}
	if !strings.Contains(words(c), "Portal") || len(c.Keys) != 2 || c.XTicks[0].Label != "Jan 2026" {
		t.Errorf("words %q, keys %+v, ticks %+v", words(c), c.Keys, c.XTicks)
	}
}

func TestAOneDayItemIsAMilestone(t *testing.T) {
	b := drawTimeline(t)
	if launch := b.Spans[1]; !launch.Milestone || launch.Said != "12 Jan 2026" {
		t.Errorf("a one-day launch is %+v, want a milestone said as its day", launch)
	}
	if b.Spans[0].Milestone || b.Spans[2].Milestone {
		t.Errorf("a phase of weeks was taken for a milestone: %+v", b.Spans)
	}
	c := run.Printable(run.View{Blocks: []run.Block{b}})[0]
	if dots := marksOf(c, document.DotMark); len(dots) != 1 || dots[0].Shape != document.DiamondShape {
		t.Errorf("the milestone printed as %+v, want one diamond", dots)
	}
}

func TestATimelineDrawsTodayWhereTheCalendarReachesIt(t *testing.T) {
	on := func(day string) func() time.Time {
		return func() time.Time { d, _ := time.Parse("2006-01-02", day); return d.Add(15 * time.Hour) }
	}
	b := drawPlan(t, "          today: true\n", on("2026-03-10"))
	if b.Today == nil || b.Today.Formatted != "10 Mar" || b.Today.Value <= b.Spans[0].From || b.Today.Value >= b.Spans[0].To {
		t.Fatalf("today is %+v, want 10 Mar inside the warehouse's bar", b.Today)
	}
	c := run.Printable(run.View{Blocks: []run.Block{b}})[0]
	if !strings.Contains(words(c), "Today · 10 Mar") {
		t.Errorf("the printed plan says %q, want today on it", words(c))
	}
	if b := drawPlan(t, "          today: true\n", on("2030-01-01")); b.Today != nil {
		t.Errorf("today past the plan's end was drawn at %+v", b.Today)
	}
	if b := drawPlan(t, "", on("2026-03-10")); b.Today != nil {
		t.Errorf("today was drawn on a plan that did not ask: %+v", b.Today)
	}
}
