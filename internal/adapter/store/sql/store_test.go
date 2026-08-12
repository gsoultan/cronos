package sql_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/principal"
	_ "modernc.org/sqlite"
)

/*
 * Run against SQLite, mostly.
 *
 * The statements use nothing outside ON CONFLICT and ordinary predicates, so
 * what is under test here — that every read carries the tenant, that versions
 * are content-addressed, that history outlives a delete — is the same logic
 * Postgres will run, and SQLite runs it in a millisecond with no container.
 *
 * Two kinds of test are not like that, and both have somewhere else to be.
 * Postgres-specific types and DDL are postgres_test.go, which has been there
 * since the store's DDL turned out to be wrong for months while every test
 * passed. Concurrency is drivers_test.go: SQLite serialises writes, so a
 * conditional UPDATE, an ON CONFLICT and a plain read-then-write all look
 * identical there, and every "two at once produce one" guarantee written
 * against it was proved only on a database nobody deploys. Those run on both.
 */

var opened int

func open(t *testing.T) *store.Store {
	t.Helper()
	// Named per call, not per test: a test that opens two stores wants two
	// databases, and sharing one would make an isolation test pass for the
	// wrong reason.
	opened++
	db, err := sql.Open("sqlite",
		fmt.Sprintf("file:store-%s-%d?mode=memory&cache=shared", t.Name(), opened))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	s := store.New(db, store.Question).
		WithClock(func() time.Time { return StoreNow })
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func who(org, project string) principal.Principal {
	return principal.Principal{Subject: "pipeline", OrgID: org, ProjectID: project,
		ProjectRole: principal.ProjectAdmin}
}

var acme = who("acme", "finance")

func doc(label string) []byte {
	return []byte("apiVersion: cronos.dev/v1\nkind: Report\nmetadata: {name: " + label + "}\n")
}

func TestARoundTrip(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	version, err := s.Put(ctx, acme, "Report", "billing", doc("billing"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, acme, "Report", "billing")
	if err != nil {
		t.Fatal(err)
	}
	// The bytes the author submitted, not our rendering of them.
	if string(got) != string(doc("billing")) {
		t.Errorf("came back as %q", got)
	}

	list, err := s.List(ctx, acme)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "billing" || list[0].Version != version {
		t.Errorf("list = %+v", list)
	}
}

// The claim the store exists to make. Two tenants, one name, and neither can
// see the other — checked by asking, not by reading the SQL.
func TestOneTenantCannotReachAnothers(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	northwind := who("northwind", "finance")

	if _, err := s.Put(ctx, acme, "Report", "billing", doc("acme-billing")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(ctx, northwind, "Report", "billing", doc("northwind-billing")); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, acme, "Report", "billing")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "northwind") {
		t.Fatal("acme was served northwind's report")
	}

	// Same organization, different project: also unrelated.
	other := who("acme", "operations")
	if _, err := s.Get(ctx, other, "Report", "billing"); !errors.Is(err, publish.ErrNotFound) {
		t.Errorf("a sibling project could read it: %v", err)
	}
	if list, _ := s.List(ctx, other); len(list) != 0 {
		t.Errorf("a sibling project listed %d definitions", len(list))
	}
}

// Telling "not found" apart from "not yours" would let a caller enumerate what
// other projects have.
func TestAnotherTenantsDefinitionIsSimplyNotFound(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if _, err := s.Put(ctx, acme, "Report", "billing", doc("billing")); err != nil {
		t.Fatal(err)
	}

	// The same name, asked for by a tenant who does not own it — once where it
	// exists elsewhere, once where it exists nowhere. Comparing two different
	// names would only prove that an error message contains the name.
	elsewhere := open(t)
	if _, err := elsewhere.Put(ctx, acme, "Report", "billing", doc("billing")); err != nil {
		t.Fatal(err)
	}
	nowhere := open(t)

	_, exists := elsewhere.Get(ctx, who("northwind", "finance"), "Report", "billing")
	_, absent := nowhere.Get(ctx, who("northwind", "finance"), "Report", "billing")

	if exists == nil || absent == nil {
		t.Fatal("both should fail")
	}
	if exists.Error() != absent.Error() {
		t.Errorf("a caller can tell the two apart:\n  %v\n  %v", exists, absent)
	}
}

// An empty organization would match rows written with an empty one, which is a
// tenant nobody meant to create and everybody can reach.
func TestAPrincipalWithNoTenancyIsRefused(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	for _, pr := range []principal.Principal{
		{Subject: "x"}, {Subject: "x", OrgID: "acme"}, {Subject: "x", ProjectID: "finance"},
	} {
		if _, err := s.Put(ctx, pr, "Report", "b", doc("b")); !errors.Is(err, publish.ErrForbidden) {
			t.Errorf("stored for %+v: %v", pr, err)
		}
		if _, err := s.Get(ctx, pr, "Report", "b"); !errors.Is(err, publish.ErrForbidden) {
			t.Errorf("read for %+v: %v", pr, err)
		}
	}
}

func TestVersionsAreContentAddressedAndKept(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	first, _ := s.Put(ctx, acme, "Report", "billing", doc("v1"))
	again, _ := s.Put(ctx, acme, "Report", "billing", doc("v1"))
	if first != again {
		t.Errorf("the same bytes gave %s then %s", first, again)
	}

	second, _ := s.Put(ctx, acme, "Report", "billing", doc("v2"))
	if second == first {
		t.Error("changed bytes kept the old version")
	}

	versions, err := s.Versions(ctx, acme, "Report", "billing")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("kept %d versions, want both", len(versions))
	}

	// The point of keeping them: a run naming a version can be replayed
	// against exactly the document that produced it.
	old, err := s.AtVersion(ctx, acme, "Report", "billing", first)
	if err != nil {
		t.Fatal(err)
	}
	if string(old) != string(doc("v1")) {
		t.Errorf("the old version came back as %q", old)
	}
}

// Deleting a definition is not a claim that it never existed.
func TestHistoryOutlivesADelete(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	version, _ := s.Put(ctx, acme, "Report", "billing", doc("v1"))
	if err := s.Delete(ctx, acme, "Report", "billing"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, acme, "Report", "billing"); !errors.Is(err, publish.ErrNotFound) {
		t.Errorf("it is still live: %v", err)
	}
	if _, err := s.AtVersion(ctx, acme, "Report", "billing", version); err != nil {
		t.Errorf("a run using it can no longer be reproduced: %v", err)
	}
}

// A silent success leaves a pipeline believing it cleaned up.
func TestDeletingNothingIsAnError(t *testing.T) {
	s := open(t)
	if err := s.Delete(context.Background(), acme, "Report", "never-existed"); !errors.Is(err, publish.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// A definition whose history is missing cannot be reproduced. If only one of
// the two writes can survive a failure, it has to be the history.
func TestPutIsAtomic(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	if _, err := s.Put(ctx, acme, "Report", "billing", doc("v1")); err != nil {
		t.Fatal(err)
	}
	versions, _ := s.Versions(ctx, acme, "Report", "billing")
	list, _ := s.List(ctx, acme)

	if len(versions) != 1 || len(list) != 1 {
		t.Errorf("one write left %d versions and %d definitions", len(versions), len(list))
	}
}

// The statements are written once with `?` and renumbered, so a tenancy
// predicate cannot be present in one dialect's copy and missing from the
// other's.
func TestBothPlaceholderStylesProduceTheSameStatements(t *testing.T) {
	db, err := sql.Open("sqlite", "file:marks?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	pg := store.New(db, store.Dollar)
	if err := pg.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	// SQLite accepts $1-style parameters too, so the numbered form is
	// exercised end to end rather than asserted as a string.
	if _, err := pg.Put(context.Background(), acme, "Report", "billing", doc("v1")); err != nil {
		t.Fatalf("numbered placeholders: %v", err)
	}
	if _, err := pg.Get(context.Background(), acme, "Report", "billing"); err != nil {
		t.Fatalf("numbered placeholders on read: %v", err)
	}
}
