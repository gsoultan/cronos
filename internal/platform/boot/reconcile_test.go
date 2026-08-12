package boot

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/gsoultan/cronos/internal/adapter/store/file"
	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Which of the definitions directory and the store is the truth.
//
// The answer is the store, once it holds anything, and the consequences are
// what these check: an edit is not reverted by the next deploy, a deletion
// does not come back because its file is still on disk, and a definition the
// store has but no file ever did is live all the same.

const dataset = `apiVersion: cronos.dev/v1
kind: Dataset
metadata:
  name: invoices
  description: %s
spec:
  sources:
    - ref: warehouse
  query: SELECT 1 AS id
  fields:
    - {name: id, type: string, role: dimension}
`

func TestAnEmptyStoreAdoptsTheDirectory(t *testing.T) {
	dir := definitionsIn(t, fmt.Sprintf(dataset, "from the file"))
	repo := load(t, dir)
	store := emptyStore(t)

	if err := reconcile(context.Background(), store, repo, "acme", "finance", quiet()); err != nil {
		t.Fatal(err)
	}

	entries, err := store.List(context.Background(), deployment)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "invoices" {
		t.Fatalf("the store did not adopt the directory: %v", entries)
	}
}

// The property the whole rule exists for: publishing is not undone by a
// restart, even though the file it was seeded from still says the old thing.
func TestAPublishedEditSurvivesARestart(t *testing.T) {
	dir := definitionsIn(t, fmt.Sprintf(dataset, "from the file"))
	store := emptyStore(t)

	first := load(t, dir)
	if err := reconcile(context.Background(), store, first, "acme", "finance", quiet()); err != nil {
		t.Fatal(err)
	}
	edited := []byte(fmt.Sprintf(dataset, "edited in the portal"))
	if _, err := store.Put(context.Background(), deployment, "Dataset", "invoices", edited); err != nil {
		t.Fatal(err)
	}

	// A second process, reading the same untouched directory.
	next := load(t, dir)
	if err := reconcile(context.Background(), store, next, "acme", "finance", quiet()); err != nil {
		t.Fatal(err)
	}

	ds, err := next.Dataset(context.Background(), "invoices")
	if err != nil {
		t.Fatal(err)
	}
	if ds.Description != "edited in the portal" {
		t.Fatalf("the directory won: %q", ds.Description)
	}
}

// The other half. Consulting the directory every boot would republish the file
// of something somebody deliberately removed.
func TestADeletionIsNotResurrectedByItsFile(t *testing.T) {
	dir := definitionsIn(t,
		fmt.Sprintf(dataset, "kept"),
		strings.Replace(fmt.Sprintf(dataset, "removed"), "name: invoices", "name: drafts", 1))
	store := emptyStore(t)

	if err := reconcile(context.Background(), store, load(t, dir), "acme", "finance", quiet()); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), deployment, "Dataset", "drafts"); err != nil {
		t.Fatal(err)
	}

	next := load(t, dir)
	if err := reconcile(context.Background(), store, next, "acme", "finance", quiet()); err != nil {
		t.Fatal(err)
	}
	if _, err := next.Dataset(context.Background(), "drafts"); err == nil {
		t.Fatal("a deleted definition came back because its file was still there")
	}
	if _, err := next.Dataset(context.Background(), "invoices"); err != nil {
		t.Fatalf("the one nobody deleted went missing: %v", err)
	}
}

// A store-only definition runs. Nothing on disk describes it, and after the
// first publish that is the ordinary case rather than the exotic one.
func TestADefinitionNoFileEverHadIsLive(t *testing.T) {
	dir := definitionsIn(t, fmt.Sprintf(dataset, "from the file"))
	store := emptyStore(t)
	if err := reconcile(context.Background(), store, load(t, dir), "acme", "finance", quiet()); err != nil {
		t.Fatal(err)
	}

	only := strings.Replace(fmt.Sprintf(dataset, "published only"), "name: invoices", "name: refunds", 1)
	if _, err := store.Put(context.Background(), deployment, "Dataset", "refunds", []byte(only)); err != nil {
		t.Fatal(err)
	}

	next := load(t, dir)
	if err := reconcile(context.Background(), store, next, "acme", "finance", quiet()); err != nil {
		t.Fatal(err)
	}
	if _, err := next.Dataset(context.Background(), "refunds"); err != nil {
		t.Fatalf("a definition only the store had did not run: %v", err)
	}
}

var deployment = principal.Principal{
	Subject: "cronosd", OrgID: "acme", ProjectID: "finance",
	ProjectRole: principal.ProjectEditor, Member: true,
}

func definitionsIn(t *testing.T, docs ...string) string {
	t.Helper()
	dir := t.TempDir()
	for i, doc := range docs {
		path := filepath.Join(dir, fmt.Sprintf("%d.yaml", i))
		if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func load(t *testing.T, dir string) *file.Repository {
	t.Helper()
	repo, err := file.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

var stores int

func emptyStore(t *testing.T) *sqlstore.Store {
	t.Helper()
	stores++
	// Shared cache, because database/sql pools connections and each connection
	// to a plain in-memory SQLite would get its own empty database.
	db, err := sql.Open("sqlite",
		fmt.Sprintf("file:reconcile-%s-%d?mode=memory&cache=shared", t.Name(), stores))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	s := sqlstore.New(db, sqlstore.Question)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
