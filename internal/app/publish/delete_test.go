package publish_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Deleting is where a project breaks quietly.
//
// The store checks the tenant and nothing else, so every rule about who may
// remove what and what would break lives here — and a definition removed from
// under a report fails at six in the morning on the first of the month, naming
// something that no longer exists to explain itself.

func TestAViewerMayNotDelete(t *testing.T) {
	svc, store := service(catalog{})

	err := svc.Delete(context.Background(), viewer, "Dataset", "invoices")
	if !errors.Is(err, publish.ErrForbidden) {
		t.Fatalf("a viewer deleted a definition: %v", err)
	}
	if store.deleted != "" {
		t.Fatalf("it reached the store anyway: %s", store.deleted)
	}
}

func TestADatasetAReportReadsIsNotDeleted(t *testing.T) {
	svc, store := service(catalog{
		reports: []definition.Report{{Name: "billing", Dataset: "invoices"}},
	})

	err := svc.Delete(context.Background(), editor, "Dataset", "invoices")
	if !errors.Is(err, publish.ErrInUse) {
		t.Fatalf("want in-use, got %v", err)
	}
	// The name, so somebody can go and fix it rather than search for it.
	if !strings.Contains(err.Error(), `report "billing"`) {
		t.Fatalf("the message does not name the report: %v", err)
	}
	if store.deleted != "" {
		t.Fatal("it was deleted anyway")
	}
}

// A block may bind to a dataset the report's own default says nothing about,
// which is the whole reason one report can combine invoices and shipments.
func TestABlockReferenceCountsAsAReference(t *testing.T) {
	svc, _ := service(catalog{
		reports: []definition.Report{{
			Name: "mixed", Dataset: "invoices",
			Outputs: []definition.Output{{
				Layout: []definition.Block{{Kind: definition.TableBlock, Dataset: "shipments"}},
			}},
		}},
	})

	if err := svc.Delete(context.Background(), editor, "Dataset", "shipments"); !errors.Is(err, publish.ErrInUse) {
		t.Fatalf("a block's dataset was deletable: %v", err)
	}
}

// A burst fans out over a dataset the report it renders knows nothing about.
func TestABurstDatasetCountsAsAReference(t *testing.T) {
	svc, _ := service(catalog{
		schedules: []definition.Schedule{{
			Name: "monthly", Report: "statement",
			Burst: &definition.BurstSpec{Over: definition.OverSpec{Dataset: "customers"}},
		}},
	})

	err := svc.Delete(context.Background(), editor, "Dataset", "customers")
	if !errors.Is(err, publish.ErrInUse) || !strings.Contains(err.Error(), `schedule "monthly"`) {
		t.Fatalf("a burst's dataset was deletable: %v", err)
	}
}

func TestASourceADatasetReadsIsNotDeleted(t *testing.T) {
	svc, _ := service(catalog{
		datasets: []definition.Dataset{{
			Name: "invoices", Sources: []definition.SourceRef{{Ref: "warehouse"}},
		}},
	})

	if err := svc.Delete(context.Background(), editor, "DataSource", "warehouse"); !errors.Is(err, publish.ErrInUse) {
		t.Fatalf("a source in use was deletable: %v", err)
	}
}

func TestWhatNothingPointsAtIsDeleted(t *testing.T) {
	svc, store := service(catalog{
		reports: []definition.Report{{Name: "billing", Dataset: "invoices"}},
	})

	if err := svc.Delete(context.Background(), editor, "Dataset", "drafts"); err != nil {
		t.Fatalf("an unreferenced dataset was refused: %v", err)
	}
	if store.deleted != "Dataset/drafts" {
		t.Fatalf("the store was asked for %q", store.deleted)
	}
}

// Every dependant, not the first one found: fixing one and being told about
// the next is the same conversation twice.
func TestEveryDependantIsNamedAtOnce(t *testing.T) {
	svc, _ := service(catalog{
		reports: []definition.Report{
			{Name: "billing", Dataset: "invoices"},
			{Name: "ageing", Dataset: "invoices"},
		},
	})

	err := svc.Delete(context.Background(), editor, "Dataset", "invoices")
	if !strings.Contains(err.Error(), `"ageing" and report "billing"`) {
		t.Fatalf("not both, in one sentence: %v", err)
	}
}

var (
	editor = principal.Principal{
		OrgID: "acme", ProjectID: "finance", ProjectRole: principal.ProjectEditor, Member: true,
	}
	viewer = principal.Principal{
		OrgID: "acme", ProjectID: "finance", ProjectRole: principal.ProjectViewer,
	}
)

type catalog struct {
	datasets  []definition.Dataset
	reports   []definition.Report
	schedules []definition.Schedule
}

func (c catalog) Datasets() []definition.Dataset   { return c.datasets }
func (c catalog) Reports() []definition.Report     { return c.reports }
func (c catalog) Schedules() []definition.Schedule { return c.schedules }

type recorder struct{ deleted string }

func (r *recorder) Put(context.Context, principal.Principal, string, string, []byte) (string, error) {
	return "", nil
}
func (r *recorder) Get(context.Context, principal.Principal, string, string) ([]byte, error) {
	return nil, publish.ErrNotFound
}
func (r *recorder) List(context.Context, principal.Principal) ([]publish.Entry, error) {
	return nil, nil
}
func (r *recorder) Delete(_ context.Context, _ principal.Principal, kind, name string) error {
	r.deleted = kind + "/" + name
	return nil
}

func service(c catalog) (*publish.Service, *recorder) {
	store := &recorder{}
	return publish.New(store, nil).WithCatalog(c), store
}
