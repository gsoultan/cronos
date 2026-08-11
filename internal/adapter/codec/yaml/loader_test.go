package yaml

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

// examples/ is the format's documentation. Loading it here makes it the
// format's *test* as well — otherwise the files drift from the code and the
// first person to notice is someone copying one that no longer works.
const examples = "../../../../examples"

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examples, path))
	if err != nil {
		t.Fatalf("the documented example is missing: %v", err)
	}
	return b
}

func TestTheShippedDatasetsLoad(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(examples, "datasets"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no example datasets — this test would pass vacuously")
	}

	for _, e := range entries {
		t.Run(e.Name(), func(t *testing.T) {
			ds, err := Loader{}.Dataset(read(t, filepath.Join("datasets", e.Name())))
			if err != nil {
				t.Fatalf("%s does not load: %v", e.Name(), err)
			}
			if ds.Name == "" || ds.Query == "" || len(ds.Fields) == 0 {
				t.Errorf("%s loaded empty: %+v", e.Name(), ds)
			}
			// Loading is not enough: it must also be compilable, which is the
			// property an author actually cares about.
			if err := query.Check(ds); err != nil {
				t.Errorf("%s will not compile: %v", e.Name(), err)
			}
		})
	}
}

func TestTheInvoicesDatasetIsReadInFull(t *testing.T) {
	ds, err := Loader{}.Dataset(read(t, "datasets/invoices.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	if ds.Name != "invoices" {
		t.Errorf("name = %q, want invoices — metadata.name is the identity", ds.Name)
	}
	if got := len(ds.Params); got != 4 {
		t.Errorf("got %d params, want 4", got)
	}
	if p, ok := ds.Param("status"); !ok || p.Type != definition.Enum || !p.Multiple {
		t.Errorf("status param = %+v, want a multiple enum", p)
	}
	if f, ok := ds.Field("total"); !ok || f.Role != definition.Measure || f.CurrencyField != "currency" {
		t.Errorf("total field = %+v, want a measure carrying its currency", f)
	}
	if len(ds.RowLevelSecurity) != 1 ||
		!strings.Contains(ds.RowLevelSecurity[0].Predicate, ".scope.customer_id") {
		t.Errorf("row scope = %+v, want the customer predicate", ds.RowLevelSecurity)
	}
}

func TestKindRoutesWithoutDecodingTheSpec(t *testing.T) {
	for path, want := range map[string]string{
		"datasets/invoices.yaml":                     KindDataset,
		"datasources/warehouse.yaml":                 KindDataSource,
		"reports/monthly-invoice-statement.yaml":     KindReport,
		"schedules/monthly-customer-statements.yaml": KindSchedule,
	} {
		got, err := Loader{}.Kind(read(t, path))
		if err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if got != want {
			t.Errorf("%s is a %q, want %q", path, got, want)
		}
	}
}

func TestDecodeRefuses(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		says string
	}{
		// A file written for a later version may parse cleanly and mean
		// something else. Refusing beats producing the wrong report.
		{"a version this build does not know", `
apiVersion: cronos.dev/v2
kind: Dataset
metadata: {name: x}
spec: {}`, "apiVersion"},

		{"a document with no name", `
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {}
spec: {}`, "metadata.name is required"},

		// Decoding a Report's spec into a Dataset would drop every field
		// silently and store something empty.
		{"the wrong kind", `
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: x}
spec: {}`, "this is a Report, not a Dataset"},

		{"malformed yaml", "apiVersion: [unclosed", "cannot decode"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Loader{}.Dataset([]byte(c.doc))
			if !errors.Is(err, ErrDecode) {
				t.Fatalf("got %v, want ErrDecode", err)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("message %q does not say %q", err, c.says)
			}
		})
	}
}

// A document that parses but breaks the rules is a different failure, and the
// loader must surface it rather than hand back a broken value.
func TestValidationFailuresSurviveLoading(t *testing.T) {
	_, err := Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: invoices}
spec:
  sources: [{ref: warehouse}]
  query: SELECT 1
  fields:
    - {name: total, type: decimal, role: measure}`))

	if !errors.Is(err, definition.ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid — a measure with no aggregate", err)
	}
}

func TestTheShippedReportLoads(t *testing.T) {
	r, err := Loader{}.Report(read(t, "reports/monthly-invoice-statement.yaml"))
	if err != nil {
		t.Fatalf("the documented report does not load: %v", err)
	}

	if r.Dataset != "invoices" {
		t.Errorf("dataset = %q, want invoices", r.Dataset)
	}
	if got := len(r.Outputs); got != 3 {
		t.Fatalf("got %d outputs, want interactive, pdf and xlsx", got)
	}
	if r.Params["status"].Default == nil {
		t.Error("the report's narrowed status default was dropped")
	}

	web, ok := r.Rendered(definition.Interactive)
	if !ok {
		t.Fatal("no interactive output — nothing to embed")
	}
	if got := len(web.Layout); got != 4 {
		t.Errorf("interactive layout has %d blocks, want 4", got)
	}
	if web.Layout[0].Kind != definition.StatBlock || web.Layout[0].Value.Aggregate != "sum" {
		t.Errorf("first block = %+v, want a summed stat", web.Layout[0])
	}

	pdf, ok := r.Rendered(definition.Paginated)
	if !ok {
		t.Fatal("no paginated output")
	}
	if pdf.Page.Size != "A4" || pdf.Page.Margins != "20mm" {
		t.Errorf("page = %+v, want A4 with 20mm margins", pdf.Page)
	}
	if got := r.Datasets(); len(got) != 1 || got[0] != "invoices" {
		t.Errorf("datasets = %v, want [invoices]", got)
	}
}

// yaml.v3 ignores unknown keys by default, which turns a typo into a quietly
// different report rather than a message.
func TestATypoIsRefusedRatherThanIgnored(t *testing.T) {
	_, err := Loader{}.Dataset([]byte(`
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: invoices}
spec:
  sources: [{ref: warehouse}]
  query: SELECT 1
  feilds:
    - {name: id, type: string, role: dimension}`))

	if !errors.Is(err, ErrDecode) {
		t.Fatalf("got %v, want ErrDecode for a misspelt key", err)
	}
	if !strings.Contains(err.Error(), "feilds") {
		t.Errorf("the message should name the key that was not understood: %v", err)
	}
}

func TestTheShippedDataSourcesLoad(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(examples, "datasources"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Run(e.Name(), func(t *testing.T) {
			ds, err := Loader{}.DataSource(read(t, filepath.Join("datasources", e.Name())))
			if err != nil {
				t.Fatalf("%s does not load: %v", e.Name(), err)
			}
			// Defaulted rather than zero: a source with no statement timeout is
			// a connection pool waiting to be held by a query nobody watches.
			if ds.Limits.Timeout() <= 0 || ds.Limits.Rows() <= 0 {
				t.Errorf("%s has no effective limits: %+v", e.Name(), ds.Limits)
			}
		})
	}
}

func TestTheShippedScheduleLoads(t *testing.T) {
	s, err := Loader{}.Schedule(read(t, "schedules/monthly-customer-statements.yaml"))
	if err != nil {
		t.Fatalf("the documented schedule does not load: %v", err)
	}
	if !s.Bursts() || s.Burst.Over.Dataset != "active-customers" {
		t.Errorf("burst = %+v", s.Burst)
	}
	if s.Timezone == "" {
		t.Error(`"the first of the month" is a local claim and needs a timezone`)
	}
	if got := len(s.Deliver); got != 2 {
		t.Errorf("got %d deliveries, want email and s3", got)
	}
	// One destination field for every channel — see DeliverSpec.
	for _, d := range s.Deliver {
		if d.To == "" {
			t.Errorf("%s delivery has no destination", d.Via)
		}
	}
}

// Durations are written the way people say them, not as nanoseconds.
func TestDurationsReadAsAuthorsWriteThem(t *testing.T) {
	ds, err := Loader{}.DataSource(read(t, "datasources/events-lake.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if ds.Limits.StatementTimeout.String() != "2m0s" {
		t.Errorf("120s parsed as %s", ds.Limits.StatementTimeout)
	}
}

// Every kind carries a display name, and the loader must copy it.
//
// The name is an identifier other definitions point at; the title is what a
// person calls it. A form that asks for both and has one silently discarded is
// a form that comes back with a blank field, and a strict decoder that has
// never heard of the key refuses the document outright.
func TestTitleSurvivesEveryKind(t *testing.T) {
	load := Loader{}

	ds, err := load.Dataset([]byte(`apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: invoices, title: Invoices}
spec:
  sources: [{ref: warehouse}]
  query: SELECT 1 AS id
  fields: [{name: id, type: string, role: dimension}]
`))
	if err != nil {
		t.Fatal(err)
	}
	if ds.Title != "Invoices" || ds.Heading() != "Invoices" {
		t.Fatalf("dataset title: %q, heading %q", ds.Title, ds.Heading())
	}

	src, err := load.DataSource([]byte(`apiVersion: cronos.dev/v1
kind: DataSource
metadata: {name: warehouse, title: Production warehouse}
spec: {driver: sqlite, dsn: "file:x?mode=memory"}
`))
	if err != nil {
		t.Fatal(err)
	}
	if src.Title != "Production warehouse" {
		t.Fatalf("datasource title: %q", src.Title)
	}

	rep, err := load.Report([]byte(`apiVersion: cronos.dev/v1
kind: Report
metadata: {name: monthly, title: Monthly statement}
spec:
  dataset: invoices
  outputs:
    - name: interactive
      renderer: interactive
      layout: [{kind: table, columns: [id]}]
`))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Title != "Monthly statement" || rep.Heading() != "Monthly statement" {
		t.Fatalf("report title: %q", rep.Title)
	}

	sc, err := load.Schedule([]byte(`apiVersion: cronos.dev/v1
kind: Schedule
metadata: {name: monthly, title: Monthly send}
spec:
  report: monthly
  output: interactive
  cron: "0 6 1 * *"
  timezone: Europe/Berlin
  deliver: [{via: email, to: a@b.example}]
`))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Title != "Monthly send" {
		t.Fatalf("schedule title: %q", sc.Title)
	}
}
