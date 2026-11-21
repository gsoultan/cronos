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
A dumbbell is each category as two dots on one scale joined by a line: where
it ran from and where it ran to, so the line's length is the change.
*/

func drawDumbbell(t *testing.T) run.Block {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`
CREATE TABLE teams (team TEXT, sla REAL, took REAL);
INSERT INTO teams VALUES
  ('Billing',  24, 30), ('Billing', 24, 34),
  ('Returns',  48, 20),
  ('Accounts', 8,  8);`); err != nil {
		t.Fatal(err)
	}
	ds, err := yamlcodec.Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: teams}
spec:
  sources: [{ref: warehouse}]
  query: SELECT team, sla, took FROM teams
  fields:
    - {name: team, type: string,  role: dimension}
    - {name: sla,  type: decimal, role: measure, aggregate: avg, label: SLA (h), decimals: 1}
    - {name: took, type: decimal, role: measure, aggregate: avg, label: Resolution (h), decimals: 1}
`))
	if err != nil {
		t.Fatalf("dataset: %v", err)
	}
	rep, err := yamlcodec.Loader{}.Report([]byte(`
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: desk}
spec:
  dataset: teams
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: chart
          chart: dumbbell
          title: Resolution against SLA
          x: {field: team}
          metrics:
            - {field: sla, aggregate: avg}
            - {field: took, aggregate: avg}
`))
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	s := run.New(datasets{"teams": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db), Builder: query.NewBuilder(query.SQLite{}),
	}})
	v, err := s.Render(context.Background(), rep, run.Request{}, anyone())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return v.Blocks[0]
}

func TestADumbbellRunsFromItsFirstMetricToItsSecond(t *testing.T) {
	b := drawDumbbell(t)
	got := []string{}
	for _, d := range b.Dumbbells {
		got = append(got, d.Label)
	}
	// Largest where it ended first.
	if strings.Join(got, ",") != "Billing,Returns,Accounts" {
		t.Fatalf("rows %v, want Billing, Returns, Accounts", got)
	}
	billing, returns, accounts := b.Dumbbells[0], b.Dumbbells[1], b.Dumbbells[2]
	if billing.From != 24 || billing.To != 32 || billing.FromSaid != "24.0" || billing.ToSaid != "32.0" || billing.Change != "+8.0" {
		t.Errorf("billing is %+v", billing)
	}
	if returns.Change != "−28.0" || accounts.Change != "0.0" {
		t.Errorf("a fall reads %q and no change %q", returns.Change, accounts.Change)
	}
	if len(b.Ends) != 2 || b.Ends[0] != "SLA (h)" || b.Ends[1] != "Resolution (h)" {
		t.Errorf("the ends are %v", b.Ends)
	}
	if a := b.XAxis; a == nil || a.Min > 8 || a.Max < 48 {
		t.Errorf("the scale %+v does not reach both ends of every row", a)
	}
}

func TestADumbbellPrintsTwoDotsAndALineARow(t *testing.T) {
	c := run.Printable(run.View{Blocks: []run.Block{drawDumbbell(t)}})[0]
	if dots := marksOf(c, document.DotMark); len(dots) != 6 || dots[1].Value != "32.0" {
		t.Errorf("%d dots, the second %+v", len(dots), dots)
	}
	if !strings.Contains(words(c), "32.0 (+8.0)") || len(c.Keys) != 2 || c.Keys[0].Label != "SLA (h)" {
		t.Errorf("words %q, keys %+v", words(c), c.Keys)
	}
}
