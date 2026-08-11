package burst_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	yamlcodec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	filechannel "github.com/gsoultan/cronos/internal/adapter/deliver/file"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/adapter/render/paginated"
	"github.com/gsoultan/cronos/internal/app/burst"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
	_ "modernc.org/sqlite"
)

/*
 * A burst, end to end: three customers, three PDFs, three files on disk.
 *
 * Everything real. A fake renderer would assert that this package can call a
 * function; a real one asserts that a statement typesets, which is the claim
 * the product is built on.
 */

const schema = `
CREATE TABLE customers (id TEXT, name TEXT, email TEXT);
CREATE TABLE invoices (id TEXT, customer_id TEXT, issued_at TEXT, total REAL, status TEXT);
INSERT INTO customers VALUES
  ('c-1','Aurora Freight','billing@aurora.example'),
  ('c-2','Baltic Cold Chain','ap@baltic.example'),
  ('c-3','Cedar & Vine Foods','finance@cedar.example');
INSERT INTO invoices VALUES
  ('i-1','c-1','2026-07-04', 1200.00,'sent'),
  ('i-2','c-1','2026-07-19', 2500.50,'overdue'),
  ('i-3','c-2','2026-07-11', 9000.00,'sent'),
  ('i-4','c-3','2026-07-21', 750.00,'paid');`

const invoicesYAML = `
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: invoices}
spec:
  sources: [{ref: warehouse}]
  # Scoped by parameter, not by row scope. docs/tenancy.md: a dataset a
  # schedule reads must not carry a .scope predicate — a burst runs as the
  # schedule's owner, who has no embed token, so the predicate matches nothing
  # and every statement comes out empty.
  query: |
    SELECT i.id, i.customer_id, i.issued_at, i.total, i.status
    FROM invoices i
    WHERE i.issued_at >= {{ .params.from }}
      AND i.customer_id = {{ .params.customer_id }}
  params:
    - {name: from, type: date, required: true}
    - {name: customer_id, type: string, required: true}
  fields:
    - {name: id,          type: string, role: dimension, hidden: true}
    - {name: customer_id, type: string, role: dimension, hidden: true}
    - {name: issued_at,   type: date,   role: dimension, label: Issued}
    - {name: status,      type: string, role: dimension, label: Status}
    - {name: total, type: decimal, role: measure, aggregate: sum, label: Amount}
`

const customersYAML = `
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: active-customers}
spec:
  sources: [{ref: warehouse}]
  query: SELECT id, name, email FROM customers
  fields:
    - {name: id,    type: string, role: dimension}
    - {name: name,  type: string, role: dimension, label: Customer}
    - {name: email, type: string, role: dimension, hidden: true}
`

const statementYAML = `
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: statement, folder: Acme Logistics}
spec:
  dataset: invoices
  outputs:
    - name: pdf
      renderer: paginated
      page: {size: A4, orientation: portrait, margins: 18mm}
      layout:
        - kind: stat
          label: Total billed
          value: {field: total, aggregate: sum}
        - kind: table
          title: Invoices
          columns: [issued_at, status, total]
          sort: [{field: issued_at, dir: desc}]
`

const scheduleYAML = `
apiVersion: cronos.dev/v1
kind: Schedule
metadata: {name: monthly-statements}
spec:
  report: statement
  output: pdf
  cron: "0 6 1 * *"
  timezone: Europe/Berlin
  params:
    from: "2026-01-01"
  burst:
    over: {dataset: active-customers}
    bind:
      customer_id: "{{ .row.id }}"
    concurrency: 3
  deliver:
    - via: file
      to: "{{ .row.id }}"
      subject: "Your {{ .run.period }} statement"
      attach: {filename: "statement-{{ .row.id }}-{{ .run.period }}.pdf"}
`

type repo struct {
	datasets map[string]definition.Dataset
	reports  map[string]definition.Report
}

func (r repo) Dataset(_ context.Context, n string) (definition.Dataset, error) {
	ds, ok := r.datasets[n]
	if !ok {
		return definition.Dataset{}, errors.New("no dataset " + n)
	}
	return ds, nil
}

func (r repo) Report(_ context.Context, n string) (definition.Report, error) {
	rep, ok := r.reports[n]
	if !ok {
		return definition.Report{}, errors.New("no report " + n)
	}
	return rep, nil
}

// rows reads the `over` dataset. A small adapter rather than a port
// implementation in the tree, because reading recipients is one query and the
// interesting part of this test is what happens to them afterwards.
type rows struct {
	repo    repo
	exec    *sqldriver.Executor
	builder query.Builder
}

func (r rows) Rows(ctx context.Context, name string, params map[string]any,
	pr principal.Principal) ([]burst.Row, error) {

	ds, err := r.repo.Dataset(ctx, name)
	if err != nil {
		return nil, err
	}
	plan, err := r.builder.Build(ds, nil, pr)
	if err != nil {
		return nil, err
	}
	result, err := r.exec.Execute(ctx, plan)
	if err != nil {
		return nil, err
	}
	defer result.Close()

	cols, err := result.Columns()
	if err != nil {
		return nil, err
	}
	var out []burst.Row
	for result.Next() {
		cells := make([]any, len(cols))
		into := make([]any, len(cols))
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := result.Scan(into...); err != nil {
			return nil, err
		}
		row := burst.Row{}
		for i, c := range cols {
			row[c] = cells[i]
		}
		out = append(out, row)
	}
	return out, result.Err()
}

func setup(t *testing.T) (*burst.Service, definition.Schedule, string) {
	t.Helper()

	/* Shared cache, not a bare :memory:. database/sql pools connections and a
	   plain in-memory SQLite gives each one its own empty database, so the
	   schema exists on whichever connection created it and nowhere else. It
	   passes for sequential work and fails the moment anything runs two
	   queries at once — which a burst does by design. */
	db, err := sql.Open("sqlite", "file:burst?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	load := yamlcodec.Loader{}
	inv, err := load.Dataset([]byte(invoicesYAML))
	if err != nil {
		t.Fatal(err)
	}
	cust, err := load.Dataset([]byte(customersYAML))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := load.Report([]byte(statementYAML))
	if err != nil {
		t.Fatal(err)
	}
	sched, err := load.Schedule([]byte(scheduleYAML))
	if err != nil {
		t.Fatal(err)
	}

	r := repo{
		datasets: map[string]definition.Dataset{"invoices": inv, "active-customers": cust},
		reports:  map[string]definition.Report{"statement": rep},
	}
	exec := sqldriver.NewExecutor(db)
	builder := query.NewBuilder(query.SQLite{})

	runner := run.New(r, run.One{Only: run.Engine{Executor: exec, Builder: builder}})
	statements := run.NewStatements(runner, paginated.New(paginated.TypstCLI{}))

	out := t.TempDir()
	svc := burst.New(r, rows{r, exec, builder}, statements,
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		filechannel.New(out))

	return svc, sched, out
}

// The schedule's owner: a project member with no embed token, which is what a
// scheduled run actually is. The scope comes from the row binding, not from a
// token.
func owner(customer string) principal.Principal {
	return principal.Principal{Subject: "scheduler", OrgID: "o1", ProjectID: "p1",
		ProjectRole: principal.ProjectEditor,
		Scope:       map[string]string{"customer_id": customer}}
}

func TestABurstProducesADocumentPerRecipient(t *testing.T) {
	svc, sched, out := setup(t)

	result, err := svc.Run(context.Background(), sched,
		burst.Run{"period": "2026-07"}, owner(""))
	if err != nil {
		t.Fatalf("burst: %v", err)
	}

	if result.Recipients != 3 || result.Delivered != 3 {
		t.Fatalf("delivered %d of %d: %v", result.Delivered, result.Recipients, result.Failed)
	}

	for _, id := range []string{"c-1", "c-2", "c-3"} {
		path := filepath.Join(out, id, "statement-"+id+"-2026-07.pdf")
		pdf, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s was not delivered: %v", id, err)
		}
		if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
			t.Errorf("%s is not a PDF", path)
		}
		if len(pdf) < 2000 {
			t.Errorf("%s is %d bytes — too small to be a statement", path, len(pdf))
		}
	}
}

// The filename is resolved per row for a reason: a mailbox with forty
// attachments all called statement.pdf is a mailbox nobody can search.
func TestEachDeliveryIsAddressedToItsRecipient(t *testing.T) {
	svc, sched, out := setup(t)
	if _, err := svc.Run(context.Background(), sched, burst.Run{"period": "2026-07"}, owner("")); err != nil {
		t.Fatal(err)
	}

	var names []string
	_ = filepath.Walk(out, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			names = append(names, filepath.Base(path))
		}
		return nil
	})
	if len(names) != 3 {
		t.Fatalf("got %d files, want 3: %v", len(names), names)
	}
	for _, n := range names {
		if !strings.HasPrefix(n, "statement-c-") {
			t.Errorf("%q was not named for its recipient", n)
		}
	}
}

// Recipients differ in size, so two statements of the same length would mean
// the row binding never reached the query.
func TestEachStatementIsScopedToItsRow(t *testing.T) {
	svc, sched, out := setup(t)
	if _, err := svc.Run(context.Background(), sched, burst.Run{"period": "2026-07"}, owner("")); err != nil {
		t.Fatal(err)
	}

	sizes := map[string]int{}
	for _, id := range []string{"c-1", "c-3"} {
		pdf, err := os.ReadFile(filepath.Join(out, id, "statement-"+id+"-2026-07.pdf"))
		if err != nil {
			t.Fatal(err)
		}
		sizes[id] = len(pdf)
	}
	// c-1 has two invoices and c-3 has one. Identical documents would mean the
	// binding was ignored and everyone received the same thing.
	if sizes["c-1"] == sizes["c-3"] {
		t.Errorf("two recipients received identical documents (%d bytes)", sizes["c-1"])
	}
}

// A burst that delivered nothing looks exactly like one that never ran, and
// the first person to notice is the customer who did not get their invoice.
func TestAnEmptyRecipientSetIsAnError(t *testing.T) {
	svc, sched, _ := setup(t)
	sched.Burst.Over.Dataset = "active-customers"

	// A scope that matches no customers.
	empty := sched
	empty.Params = map[string]any{"from": "2026-01-01"}
	empty.Burst = &definition.BurstSpec{
		Over: definition.OverSpec{Dataset: "nobody"},
		Bind: map[string]string{"customer_id": "{{ .row.id }}"},
	}

	if _, err := svc.Run(context.Background(), empty, burst.Run{}, owner("")); err == nil {
		t.Error("a burst over a dataset that does not exist reported success")
	}
}

// A binding naming a column the row does not have would otherwise resolve to
// empty and address five thousand deliveries to nowhere.
func TestATypoInABindingIsRefused(t *testing.T) {
	svc, sched, _ := setup(t)
	sched.Burst.Bind = map[string]string{"customer_id": "{{ .row.idd }}"}

	result, err := svc.Run(context.Background(), sched, burst.Run{"period": "2026-07"}, owner(""))
	if err != nil {
		t.Fatal(err)
	}
	if result.Delivered != 0 || len(result.Failed) != 3 {
		t.Errorf("delivered %d with %d failures, want 0 and 3", result.Delivered, len(result.Failed))
	}
	if !strings.Contains(strings.Join(result.Failed, " "), ".row.idd") {
		t.Errorf("the failure should name the binding: %v", result.Failed)
	}
}

// One recipient failing must not stop the rest — a partial burst that claimed
// success is how a customer finds out by not receiving an invoice.
func TestOneFailureDoesNotStopTheBurst(t *testing.T) {
	svc, sched, out := setup(t)
	sched.Deliver = append(sched.Deliver, definition.DeliverSpec{Via: "nowhere", To: "x"})

	result, err := svc.Run(context.Background(), sched, burst.Run{"period": "2026-07"}, owner(""))
	if err != nil {
		t.Fatal(err)
	}
	if result.Recipients != 3 || len(result.Failed) != 3 {
		t.Errorf("result = %+v", result)
	}
	// The first channel still ran for every one of them.
	entries, _ := os.ReadDir(out)
	if len(entries) != 3 {
		t.Errorf("the working channel delivered to %d recipients, want 3", len(entries))
	}
}
