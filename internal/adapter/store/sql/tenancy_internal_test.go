package sql

import (
	"context"
	"database/sql"
	"testing"

	"github.com/gsoultan/cronos/internal/core/principal"

	_ "modernc.org/sqlite"
)

/*
The read predicate, asserted against a row nothing would create.

grants_test.go proves the write guard: an administrator cannot bind an account
from another project to their group. This proves the other half — that even
with such a row on disk, the query does not return it. Two defences, and a test
of only the first would pass on a build where the second was removed.

Internal because it plants the row directly, which needs s.db. Reaching for a
test-only accessor on Store would have meant widening the production API to
prove something about it.
*/
func TestAPlantedForeignMembershipIsNotReadBack(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", "file:tenancy-internal?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	s := New(db, Question)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	acme := principal.Principal{OrgID: "acme", ProjectID: "finance",
		Subject: "admin", ProjectRole: principal.ProjectAdmin}
	rival := principal.Principal{OrgID: "rival", ProjectID: "ops",
		Subject: "admin", ProjectRole: principal.ProjectAdmin}

	// The same group name in two projects, each with its own scope.
	if _, err := s.CreateGroup(ctx, acme, "finance", map[string]string{"region": "west"}); err != nil {
		t.Fatal(err)
	}
	theirs, err := s.CreateGroup(ctx, rival, "finance", map[string]string{"region": "east"})
	if err != nil {
		t.Fatal(err)
	}

	// A membership row binding an acme account to the rival group — the shape
	// a move used to leave behind, written here without going through the API.
	if _, err := s.db.ExecContext(ctx, s.sql(
		`INSERT INTO cronos_group_members (group_id, user_id, added_at) VALUES (?, ?, ?)`),
		theirs.ID, "usr_acme", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	groups, err := s.GroupsOf(ctx, "acme", "finance", "usr_acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Errorf("GroupsOf returned %v from another project — a grant to that "+
			"name here would have matched", groups)
	}

	scope, err := s.Confinement(ctx, "acme", "finance", "usr_acme")
	if err != nil {
		t.Fatalf("a foreign group's scope reached the confinement: %v", err)
	}
	if scope != nil {
		t.Errorf("confinement is %v, want nothing — that scope belongs to another project", scope)
	}

	// And the rival's own read still sees it, so the predicate narrows rather
	// than breaks.
	if got, _ := s.GroupsOf(ctx, "rival", "ops", "usr_acme"); len(got) != 1 {
		t.Errorf("the owning project no longer sees its own membership: %v", got)
	}
}
