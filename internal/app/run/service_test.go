package run_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
	_ "modernc.org/sqlite"
)

/*
 * End to end against a real database.
 *
 * The interesting claims — that row scope actually removes rows, that
 * aggregation happens in SQL, that a filter narrows what a stat counts — are
 * claims about what a database does with the statement. A fake executor
 * returning canned rows would assert that this package can format a number.
 */

const schema = `
CREATE TABLE invoices (
  id TEXT, customer_id TEXT, customer_name TEXT, issued_at TEXT,
  currency TEXT, total REAL, status TEXT
);
INSERT INTO invoices VALUES
  ('i1','c-1','Aurora Freight','2026-07-04','EUR', 1000, 'sent'),
  ('i2','c-1','Aurora Freight','2026-07-19','EUR', 2500, 'overdue'),
  ('i3','c-1','Aurora Freight','2026-08-02','EUR',  500, 'paid'),
  ('i4','c-2','Baltic Cold Chain','2026-07-11','EUR', 9000, 'sent'),
  ('i5','c-2','Baltic Cold Chain','2026-08-14','EUR', 4000, 'overdue');`

// A dataset over that table, written the way an author would.
const datasetYAML = `
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: invoices}
spec:
  sources: [{ref: warehouse}]
  query: |
    SELECT id, customer_id, customer_name, issued_at, currency, total, status
    FROM invoices
    WHERE issued_at >= {{ .params.from }}
  params:
    - {name: from, type: date, required: true}
  fields:
    - {name: id,            type: string,  role: dimension, hidden: true}
    - {name: customer_id,   type: string,  role: dimension, hidden: true}
    - {name: customer_name, type: string,  role: dimension, label: Customer}
    - {name: issued_at,     type: date,    role: dimension, label: Issued}
    - {name: currency,      type: string,  role: dimension, hidden: true}
    - {name: status,        type: string,  role: dimension, label: Status}
    - {name: total, type: decimal, role: measure, aggregate: sum, label: Amount}
  rowLevelSecurity:
    - predicate: customer_id = {{ .scope.customer_id }}`

const reportYAML = `
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: billing}
spec:
  dataset: invoices
  filters:
    - name: period
      label: Period
      type: date
      bind: {invoices: issued_at}
    - name: region
      label: Region
      type: string
      bind: {shipments: region}
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: stat
          label: Total billed
          value: {field: total, aggregate: sum}
        - kind: chart
          chart: bar
          title: Billed by month
          x: {field: issued_at, grain: month}
          y: {field: total, aggregate: sum}
        - kind: table
          title: Invoices
          columns: [customer_name, issued_at, status, total]
          sort: [{field: issued_at, dir: desc}]
          pageSize: 50`

type datasets map[string]definition.Dataset

func (d datasets) Dataset(_ context.Context, name string) (definition.Dataset, error) {
	ds, ok := d[name]
	if !ok {
		return definition.Dataset{}, errors.New("no such dataset: " + name)
	}
	return ds, nil
}

func setup(t *testing.T) (*run.Service, definition.Report) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:run?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	ds, err := yamlcodec.Loader{}.Dataset([]byte(datasetYAML))
	if err != nil {
		t.Fatalf("dataset: %v", err)
	}
	rep, err := yamlcodec.Loader{}.Report([]byte(reportYAML))
	if err != nil {
		t.Fatalf("report: %v", err)
	}

	return run.New(datasets{"invoices": ds}, run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db),
		Builder:  query.NewBuilder(query.SQLite{}),
	}}), rep
}

func customer(id string) principal.Principal {
	return principal.Principal{Subject: "u1", OrgID: "o1", ProjectID: "p1",
		ProjectRole: principal.ProjectViewer, Scope: map[string]string{"customer_id": id}}
}

func render(t *testing.T, s *run.Service, r definition.Report, req run.Request,
	pr principal.Principal) run.View {
	t.Helper()
	if req.Params == nil {
		req.Params = map[string]any{"from": "2026-01-01"}
	}
	v, err := s.Render(context.Background(), r, req, pr)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return v
}

func TestAReportRendersAgainstARealDatabase(t *testing.T) {
	s, rep := setup(t)
	v := render(t, s, rep, run.Request{}, customer("c-1"))

	if v.Title != "billing" || len(v.Blocks) != 3 {
		t.Fatalf("view = %+v", v)
	}
	// c-1 has 1000 + 2500 + 500. c-2's 13,000 must not be in it.
	if v.Blocks[0].Value != "4,000" {
		t.Errorf("stat = %q, want 4,000 — c-2's invoices leaked in", v.Blocks[0].Value)
	}
	if got := len(v.Blocks[1].Series); got != 2 {
		t.Errorf("chart has %d buckets, want July and August", got)
	}
	if got := len(v.Blocks[2].Rows); got != 3 {
		t.Errorf("table has %d rows, want c-1's three", got)
	}
}

// The claim the whole design rests on, checked against a database rather than
// against a string.
func TestRowScopeRemovesRowsInPractice(t *testing.T) {
	s, rep := setup(t)

	one := render(t, s, rep, run.Request{}, customer("c-1"))
	two := render(t, s, rep, run.Request{}, customer("c-2"))

	if one.Blocks[0].Value == two.Blocks[0].Value {
		t.Fatalf("two customers saw the same total: %q", one.Blocks[0].Value)
	}
	if two.Blocks[0].Value != "13,000" {
		t.Errorf("c-2's total = %q, want 13,000", two.Blocks[0].Value)
	}
	for _, row := range one.Blocks[2].Rows {
		if strings.Contains(strings.Join(row, " "), "Baltic") {
			t.Errorf("c-1 can see c-2's invoice: %v", row)
		}
	}
}

// docs/tenancy.md: no scope means the predicate matches nothing. Not
// everything.
func TestAScopelessCallerSeesNothing(t *testing.T) {
	s, rep := setup(t)
	member := principal.Principal{Subject: "u2", OrgID: "o1", ProjectID: "p1",
		ProjectRole: principal.ProjectEditor}

	v := render(t, s, rep, run.Request{}, member)

	if v.Blocks[0].Value != "—" {
		t.Errorf("stat = %q, want an em dash — nothing matched", v.Blocks[0].Value)
	}
	if len(v.Blocks[2].Rows) != 0 {
		t.Errorf("table returned %d rows to a caller with no scope", len(v.Blocks[2].Rows))
	}
}

func TestASharedFilterNarrowsWhatTheStatCounts(t *testing.T) {
	s, rep := setup(t)

	all := render(t, s, rep, run.Request{}, customer("c-1"))
	july := render(t, s, rep, run.Request{
		Filters: map[string]query.FilterValue{
			"period": {Op: query.Between, Values: []any{"2026-07-01", "2026-07-31"}},
		},
	}, customer("c-1"))

	if all.Blocks[0].Value != "4,000" || july.Blocks[0].Value != "3,500" {
		t.Errorf("unfiltered %q, July %q — want 4,000 and 3,500",
			all.Blocks[0].Value, july.Blocks[0].Value)
	}
	if len(july.Blocks[1].Series) != 1 {
		t.Errorf("the chart should be down to one bucket, got %d", len(july.Blocks[1].Series))
	}
}

// A filter bound to a dataset this block does not read must be reported, not
// silently skipped — the viewer promises to say so on the block.
func TestCoverageReachesTheView(t *testing.T) {
	s, rep := setup(t)
	v := render(t, s, rep, run.Request{}, customer("c-1"))

	cov := v.Blocks[0].Coverage
	if cov == nil {
		t.Fatal("no coverage on the block")
	}
	if strings.Join(cov.Applied, ",") != "period" {
		t.Errorf("applied = %v, want period", cov.Applied)
	}
	if strings.Join(cov.Ignored, ",") != "region" {
		t.Errorf("ignored = %v, want region — it binds to shipments", cov.Ignored)
	}
}

func TestTableLabelsAndAlignmentComeFromTheDataset(t *testing.T) {
	s, rep := setup(t)
	table := render(t, s, rep, run.Request{}, customer("c-1")).Blocks[2]

	if table.Columns[0].Label != "Customer" {
		t.Errorf("heading = %q, want the field's label", table.Columns[0].Label)
	}
	if table.Columns[3].Align != "right" {
		t.Errorf("a measure column should be right aligned, got %q", table.Columns[3].Align)
	}
	if table.Columns[1].Align == "right" {
		t.Error("a date is not a measure")
	}
}

// A caller shown a report that does not match what they asked for should be
// told why.
func TestAPinnedParameterIsRefusedRatherThanIgnored(t *testing.T) {
	s, rep := setup(t)
	rep.Params = map[string]definition.ParamOverride{
		"from": {Default: "2026-01-01", Pin: true},
	}

	_, err := s.Render(context.Background(), rep,
		run.Request{Params: map[string]any{"from": "2020-01-01"}}, customer("c-1"))

	if !errors.Is(err, run.ErrPinned) {
		t.Fatalf("got %v, want ErrPinned", err)
	}
}

func TestAnUnknownOutputIsAnError(t *testing.T) {
	s, rep := setup(t)
	_, err := s.Render(context.Background(), rep,
		run.Request{Output: "pdf", Params: map[string]any{"from": "2026-01-01"}}, customer("c-1"))

	if !errors.Is(err, run.ErrNotRenderable) {
		t.Fatalf("got %v, want ErrNotRenderable", err)
	}
}
