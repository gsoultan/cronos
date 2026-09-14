package sql_test

import (
	"context"
	"testing"
	"time"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
)

func orgPerson(t *testing.T, s *store.Store, id, org, project string) {
	t.Helper()
	if err := s.CreateUser(context.Background(), identity.User{
		ID: id, Email: id + "@" + org + ".example", Org: org, Project: project, Role: "viewer",
	}, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
}

// cutAt is when this account's sessions were last drawn a line under. Read
// through Active, which is what the request path consults.
func cutAt(t *testing.T, s *store.Store, id string) time.Time {
	t.Helper()
	_, _, since := s.Active(context.Background(), id)
	return since
}

func TestAnOrgRoleIsGrantedReadAndTakenAway(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	orgPerson(t, s, "usr_ada", "acme", "finance")

	if got, err := s.OrgRoleOf(ctx, "usr_ada"); err != nil || got != principal.None {
		t.Fatalf("a fresh account holds %q (%v), want none — an upgrade must "+
			"change nothing for anybody", got, err)
	}

	if err := s.GrantOrgRole(ctx, "usr_ada", principal.OrgAdmin, "usr_root"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.OrgRoleOf(ctx, "usr_ada"); got != principal.OrgAdmin {
		t.Fatalf("granted admin, holds %q", got)
	}

	// Changing it is a correction, not an error, and lands on the new value.
	if err := s.GrantOrgRole(ctx, "usr_ada", principal.OrgOwner, "usr_root"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.OrgRoleOf(ctx, "usr_ada"); got != principal.OrgOwner {
		t.Fatalf("regranted owner, holds %q", got)
	}

	if err := s.RevokeOrgRole(ctx, "usr_ada"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.OrgRoleOf(ctx, "usr_ada"); got != principal.None {
		t.Fatalf("revoked, still holds %q", got)
	}
	// Absent is success: the end state is the same.
	if err := s.RevokeOrgRole(ctx, "usr_ada"); err != nil {
		t.Errorf("revoking twice was an error: %v", err)
	}
}

// A role this build does not know is no grant. A row written by a later
// version is one this version cannot honour, and failing the sign-in over it
// would take somebody's access away for a spelling it will understand after an
// upgrade.
func TestAnOrgRoleThisBuildDoesNotKnowGrantsNothing(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	orgPerson(t, s, "usr_ada", "acme", "finance")

	if err := s.GrantOrgRole(ctx, "usr_ada", principal.Role("superowner"), "usr_root"); err == nil {
		t.Fatal("stored a role that is not one")
	}
	if got, _ := s.OrgRoleOf(ctx, "usr_ada"); got != principal.None {
		t.Errorf("it landed anyway as %q", got)
	}
}

func TestGrantingToNobodyIsRefused(t *testing.T) {
	s := open(t)
	if err := s.GrantOrgRole(context.Background(), "usr_ghost",
		principal.OrgAdmin, "usr_root"); err == nil {
		t.Fatal("granted an organization role to an account that does not exist")
	}
}

/*
TestChangingAnOrgRoleCutsTheSessions is the half that makes a revocation one.

The role travels in the token so that a request does not ask the database for
it, which means a revoked org admin keeps administration of every project in
the organization until the token expires — up to a working day. Cutting the
sessions is both simpler than checking per request and the stronger answer:
they are signed out rather than signed in with more than they hold.

Granting cuts them too, because a grant can be a reduction: owner to member is
a smaller role, and a session minted a minute earlier still carries the larger
one.
*/
func TestChangingAnOrgRoleCutsTheSessions(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	orgPerson(t, s, "usr_ada", "acme", "finance")

	for _, step := range []struct {
		name string
		do   func() error
	}{
		{"granting", func() error { return s.GrantOrgRole(ctx, "usr_ada", principal.OrgOwner, "r") }},
		{"reducing", func() error { return s.GrantOrgRole(ctx, "usr_ada", principal.OrgMember, "r") }},
		{"revoking", func() error { return s.RevokeOrgRole(ctx, "usr_ada") }},
	} {
		t.Run(step.name, func(t *testing.T) {
			before := cutAt(t, s, "usr_ada")
			if err := step.do(); err != nil {
				t.Fatal(err)
			}
			after := cutAt(t, s, "usr_ada")
			if after.IsZero() {
				t.Fatalf("%s left every session it invalidated still working", step.name)
			}
			if !before.IsZero() && !after.After(before) && !after.Equal(before) {
				t.Errorf("%s moved the line backwards", step.name)
			}
		})
	}
}

// ProjectsInOrg is what bounds an administrator entering another project, so
// it must not answer with another organization's.
func TestProjectsInOrgStopsAtTheOrganization(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	orgPerson(t, s, "usr_ada", "acme", "finance")
	orgPerson(t, s, "usr_bo", "acme", "ops")
	orgPerson(t, s, "usr_cy", "globex", "secret")

	got, err := s.ProjectsInOrg(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "finance" || got[1] != "ops" {
		t.Fatalf("acme has %v, want [finance ops]", got)
	}
	for _, p := range got {
		if p == "secret" {
			t.Fatal("it listed another organization's project")
		}
	}
}
