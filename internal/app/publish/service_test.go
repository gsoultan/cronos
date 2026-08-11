package publish_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/store/file"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

const dataset = `
apiVersion: cronos.dev/v1
kind: Dataset
metadata: {name: invoices}
spec:
  sources: [{ref: warehouse}]
  query: |
    SELECT id, customer_id, total, issued_at, status
    FROM invoices WHERE issued_at >= {{ .params.from }}
  params:
    - {name: from, type: date, required: true}
  fields:
    - {name: id,          type: string,  role: dimension}
    - {name: customer_id, type: string,  role: dimension, hidden: true}
    - {name: issued_at,   type: date,    role: dimension, label: Issued}
    - {name: status,      type: string,  role: dimension}
    - {name: total, type: decimal, role: measure, aggregate: sum, label: Amount}
  rowLevelSecurity:
    - predicate: customer_id = {{ .scope.customer_id }}
`

const report = `
apiVersion: cronos.dev/v1
kind: Report
metadata: {name: billing}
spec:
  dataset: invoices
  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: stat
          label: Total
          value: {field: total, aggregate: sum}
        - kind: chart
          chart: bar
          title: By month
          x: {field: issued_at, grain: month}
          y: {field: total, aggregate: sum}
`

func setup(t *testing.T) (*publish.Service, *file.Repository, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := file.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	writer := file.NewWriter(dir, repo)
	return publish.New(writer, repo), repo, dir
}

func admin() principal.Principal {
	return principal.Principal{Subject: "pipeline", OrgID: "o1", ProjectID: "p1",
		ProjectRole: principal.ProjectAdmin}
}

func mustPublish(t *testing.T, s *publish.Service, doc string) publish.Result {
	t.Helper()
	r, err := s.Publish(context.Background(), []byte(doc), admin())
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return r
}

func TestPublishingMakesADefinitionLive(t *testing.T) {
	s, repo, dir := setup(t)

	ds := mustPublish(t, s, dataset)
	if ds.Kind != "Dataset" || ds.Name != "invoices" {
		t.Errorf("result = %+v", ds)
	}
	// The running server serves it without a restart.
	if _, err := repo.Dataset(context.Background(), "invoices"); err != nil {
		t.Errorf("published dataset is not live: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "datasets", "invoices.yaml")); err != nil {
		t.Errorf("nothing was written: %v", err)
	}
}

// Re-publishing unchanged bytes must give the same version, or a run record
// naming one cannot be replayed against the document that produced it.
func TestVersionsAreContentAddressed(t *testing.T) {
	s, _, _ := setup(t)

	first := mustPublish(t, s, dataset)
	again := mustPublish(t, s, dataset)
	if first.Version != again.Version {
		t.Errorf("same bytes gave %s then %s", first.Version, again.Version)
	}

	changed := mustPublish(t, s, strings.Replace(dataset, "label: Amount", "label: Total", 1))
	if changed.Version == first.Version {
		t.Error("a changed document kept the old version")
	}
}

// Deleting a definition is not a claim that it never existed: a run that used
// it must still be reproducible.
func TestHistorySurvivesAPublish(t *testing.T) {
	s, _, dir := setup(t)
	mustPublish(t, s, dataset)
	mustPublish(t, s, strings.Replace(dataset, "label: Amount", "label: Total", 1))

	kept, err := filepath.Glob(filepath.Join(dir, ".versions", "Dataset", "invoices", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 2 {
		t.Errorf("kept %d versions, want both", len(kept))
	}
}

// The version directory lives inside the definitions directory, so a loader
// that walked into it would serve every historical copy as live.
func TestHistoryIsNotLoadedAsDefinitions(t *testing.T) {
	s, _, dir := setup(t)
	mustPublish(t, s, dataset)
	mustPublish(t, s, strings.Replace(dataset, "label: Amount", "label: Total", 1))

	fresh, err := file.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(fresh.Names("Dataset")); got != 1 {
		t.Errorf("loaded %d datasets, want 1 — history is being served", got)
	}
}

func TestPublishRefuses(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want error
		says string
	}{
		{"a dataset that will not validate",
			strings.Replace(dataset, "aggregate: sum, label: Amount", "label: Amount", 1),
			definition.ErrInvalid, "declares no aggregate"},

		// A report naming a dataset nobody has renders nothing, and finding
		// out at publish beats finding out when a customer opens the page.
		{"a report reading a dataset that does not exist",
			strings.Replace(report, "dataset: invoices", "dataset: nowhere", 1),
			publish.ErrNotFound, `reads dataset "nowhere"`},

		{"a report measuring a field the dataset does not publish",
			strings.Replace(report, "{field: total, aggregate: sum}\n        - kind: chart",
				"{field: profit, aggregate: sum}\n        - kind: chart", 1),
			query.ErrBadTemplate, `"profit" is not a field`},

		// Datasources are read by the driver registry, not stored here.
		{"a kind this build does not store",
			strings.Replace(dataset, "kind: Dataset", "kind: DataSource", 1),
			publish.ErrUnsupported, "DataSource"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _, _ := setup(t)
			mustPublish(t, s, dataset) // so the dataset exists for report cases
			_, err := s.Publish(context.Background(), []byte(c.doc), admin())
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("message %q does not say %q", err, c.says)
			}
		})
	}
}

// A publish-time check that compiled against an empty scope would exercise the
// FALSE substitution instead of the predicate, and a predicate that cannot
// compile would pass review.
func TestTheCheckCompilesTheRealRowScope(t *testing.T) {
	s, _, _ := setup(t)
	mustPublish(t, s, dataset)

	broken := strings.Replace(dataset,
		"predicate: customer_id = {{ .scope.customer_id }}",
		"predicate: customer_id = {{ .params.nope }}", 1)

	if _, err := s.Publish(context.Background(), []byte(broken), admin()); err == nil {
		t.Error("a predicate reading an undeclared parameter was accepted")
	}
}

func TestOnlyEditorsMayPublish(t *testing.T) {
	s, _, _ := setup(t)
	viewer := principal.Principal{Subject: "u1", OrgID: "o1", ProjectID: "p1",
		ProjectRole: principal.ProjectViewer}

	if _, err := s.Publish(context.Background(), []byte(dataset), viewer); !errors.Is(err, publish.ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
}

const schedule = `
apiVersion: cronos.dev/v1
kind: Schedule
metadata: {name: monthly}
spec:
  report: billing
  output: interactive
  cron: "0 6 1 * *"
  timezone: Europe/Berlin
  burst:
    over: {dataset: invoices}
    bind: {from: "{{ .row.issued_at }}"}
  deliver:
    - via: file
      to: "{{ .row.id }}"
`

// docs/tenancy.md sets out this rule and then says it is easy to get wrong.
// This is the sentence turned into an error: a burst runs as the schedule's
// owner, who has no embed token, so a scoped dataset matches nothing and the
// run delivers five thousand empty documents while reporting success.
func TestASchedulesDatasetsMayNotBeRowScoped(t *testing.T) {
	s, repo, _ := setup(t)
	s = s.WithReports(repo)
	mustPublish(t, s, dataset) // this one carries row-level security
	mustPublish(t, s, report)

	_, err := s.Publish(context.Background(), []byte(schedule), admin())
	if !errors.Is(err, publish.ErrScopedBySchedule) {
		t.Fatalf("got %v, want ErrScopedBySchedule", err)
	}
	if !strings.Contains(err.Error(), "every document comes out empty") {
		t.Errorf("the message should say what would happen: %v", err)
	}
}

func TestASchedulePublishesOnceItsDatasetIsParameterScoped(t *testing.T) {
	s, repo, _ := setup(t)
	s = s.WithReports(repo)

	// The same dataset, scoped by a parameter the schedule binds.
	unscoped := strings.Replace(dataset,
		"  rowLevelSecurity:\n    - predicate: customer_id = {{ .scope.customer_id }}\n", "", 1)
	mustPublish(t, s, unscoped)
	mustPublish(t, s, report)

	if _, err := s.Publish(context.Background(), []byte(schedule), admin()); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestAScheduleRunningAReportNobodyHasIsRefused(t *testing.T) {
	s, repo, _ := setup(t)
	s = s.WithReports(repo)

	_, err := s.Publish(context.Background(),
		[]byte(strings.Replace(schedule, "report: billing", "report: ghost", 1)), admin())
	if !errors.Is(err, publish.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
