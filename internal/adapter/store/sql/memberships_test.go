package sql_test

import (
	"context"
	"errors"
	"testing"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
TestAnUpgradeMakesEverybodyAMemberOfWhereTheyAlreadyAre is the migration's
whole obligation.

Backfilled from the account rows, so a deployment that upgrades finds every
account already a member of the project it was in. Without it the first sign-in
after the upgrade offers nobody anywhere to go — which is not a new feature
arriving, it is the existing one disappearing.
*/
func TestAnUpgradeMakesEverybodyAMemberOfWhereTheyAlreadyAre(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	orgPerson(t, s, "usr_ada", "acme", "finance")

	got, err := s.MembershipsOf(ctx, "usr_ada")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("a new account holds %d memberships, want the one it was created in", len(got))
	}
	if got[0].Org != "acme" || got[0].Project != "finance" || got[0].Role != "viewer" {
		t.Fatalf("got %+v, want acme/finance as a viewer", got[0])
	}
}

// A second project is a second row, with its own role — an editor in one and a
// viewer in another is the ordinary case.
func TestAMembershipCarriesItsOwnRole(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	orgPerson(t, s, "usr_ada", "acme", "finance")

	if err := s.GrantMembership(ctx, "usr_ada", "acme", "ops", "editor", "usr_root"); err != nil {
		t.Fatal(err)
	}
	role, ok, err := s.MembershipIn(ctx, "usr_ada", "acme", "ops")
	if err != nil || !ok {
		t.Fatalf("the grant did not take: ok=%v err=%v", ok, err)
	}
	if role != "editor" {
		t.Errorf("role in ops is %q, want editor", role)
	}
	// And the one they started in is untouched by it.
	if role, _, _ := s.MembershipIn(ctx, "usr_ada", "acme", "finance"); role != "viewer" {
		t.Errorf("the original membership became %q — one role is being shared", role)
	}
}

/*
TestGrantingTwiceCorrectsTheRole covers the administrator fixing a mistake.

Promoting a viewer to an editor is the same act as adding them, and a conflict
there reads as "already done" when it is not — so the second grant changes the
role rather than failing.
*/
func TestGrantingTwiceCorrectsTheRole(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	orgPerson(t, s, "usr_ada", "acme", "finance")

	if err := s.GrantMembership(ctx, "usr_ada", "acme", "ops", "viewer", "usr_root"); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantMembership(ctx, "usr_ada", "acme", "ops", "admin", "usr_root"); err != nil {
		t.Fatalf("correcting a role failed: %v", err)
	}
	if role, _, _ := s.MembershipIn(ctx, "usr_ada", "acme", "ops"); role != "admin" {
		t.Errorf("role is %q, want admin", role)
	}
	all, _ := s.MembershipsOf(ctx, "usr_ada")
	if len(all) != 2 {
		t.Errorf("granting twice made %d memberships, want 2", len(all))
	}
}

// Revoking removes the way in and leaves the account alone. Somebody removed
// from a second project is still the account they were in the first.
func TestRevokingAMembershipLeavesTheAccount(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	orgPerson(t, s, "usr_ada", "acme", "finance")

	if err := s.GrantMembership(ctx, "usr_ada", "acme", "ops", "editor", "usr_root"); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeMembership(ctx, "usr_ada", "acme", "ops"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.MembershipIn(ctx, "usr_ada", "acme", "ops"); ok {
		t.Error("the membership survived its revocation")
	}
	if u, err := s.Me(ctx, "usr_ada"); err != nil || u.Email == "" {
		t.Errorf("the account went with it: %v %+v", err, u)
	}
	if _, ok, _ := s.MembershipIn(ctx, "usr_ada", "acme", "finance"); !ok {
		t.Error("revoking one project took the other")
	}
}

// A role this code does not know is refused rather than stored. Query
// compilation reads these by name, so a fourth spelling is a grant that
// silently does nothing.
func TestAnUnknownProjectRoleIsRefused(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	orgPerson(t, s, "usr_ada", "acme", "finance")

	err := s.GrantMembership(ctx, "usr_ada", "acme", "ops", "superuser", "usr_root")
	if !errors.Is(err, store.ErrNoProjectRole) {
		t.Fatalf("got %v, want ErrNoProjectRole", err)
	}
}

// And a membership for somebody who does not exist. The grant would otherwise
// sit there waiting for an account that may never be created with that id.
func TestAMembershipNeedsAnAccount(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	err := s.GrantMembership(context.Background(), "usr_nobody", "acme", "ops", "editor", "usr_root")
	if !errors.Is(err, identity.ErrNoUser) {
		t.Fatalf("got %v, want ErrNoUser", err)
	}
	_ = ctx
}
