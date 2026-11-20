package run_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

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

func drawTimeline(t *testing.T) run.Block {
	t.Helper()
	db, err := sql.Open("sqlite", "file:timeline?mode=memory&cache=shared")
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
`))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	s := run.New(datasets{"phases": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
	}})
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
	if launch := b.Spans[1]; launch.To-launch.From != 1 || launch.Said != "12 Jan – 12 Jan 2026 · 1 day" {
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
	if len(bars) != 3 || bars[0].Value != "5 Jan – 27 Mar 2026 · 12 weeks" {
		t.Errorf("%d bars, the first %+v", len(bars), bars)
	}
	if !strings.Contains(words(c), "Portal") || len(c.Keys) != 2 || c.XTicks[0].Label != "Jan 2026" {
		t.Errorf("words %q, keys %+v, ticks %+v", words(c), c.Keys, c.XTicks)
	}
}
