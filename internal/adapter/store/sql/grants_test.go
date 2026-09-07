package sql_test

import (
	"context"
	"errors"
	"testing"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/access"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Groups, grants, and the scope a person reads through.

Two properties are worth more than the rest of this file. Every write is scoped
by org and project in the statement rather than by the caller remembering to,
so a group id from another tenant names nothing here. And a scope that cannot
be read is an error rather than an empty map — empty means "confines nothing",
so a malformed value read as empty would lift a confinement somebody set.
*/

func group(t *testing.T, s *store.Store, pr principal.Principal,
	name string, scope map[string]string) store.Group {

	t.Helper()
	g, err := s.CreateGroup(context.Background(), pr, name, scope)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	return g
}

/* -- confinement ----------------------------------------------------------- */

func TestSomebodyInNoGroupIsConfinedByNothing(t *testing.T) {
	s := open(t)

	got, err := s.Confinement(context.Background(), "u-1")
	if err != nil {
		t.Fatal(err)
	}
	// Nil, not an empty map: principal.RowScoped counts entries, and an empty
	// map must not read as a confinement.
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestAGroupConfinesItsMembers(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	g := group(t, s, acme, "west", map[string]string{"region": "west"})
	if err := s.AddToGroup(ctx, acme, g.ID, "u-1"); err != nil {
		t.Fatal(err)
	}

	got, err := s.Confinement(ctx, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if got["region"] != "west" {
		t.Fatalf("confinement is %v, want region=west", got)
	}
	// And somebody who is not in it is not confined by it.
	if other, _ := s.Confinement(ctx, "u-2"); other != nil {
		t.Errorf("a non-member was confined: %v", other)
	}
}

// A person's own scope overrides their groups' rather than merging with it.
// Two sources for one answer is how somebody reads a region nobody granted.
func TestAPersonsOwnScopeOverridesTheirGroups(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	g := group(t, s, acme, "west", map[string]string{"region": "west"})
	if err := s.AddToGroup(ctx, acme, g.ID, "u-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserScope(ctx, acme, "u-1", map[string]string{"region": "east"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Confinement(ctx, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if got["region"] != "east" {
		t.Fatalf("confinement is %v, want the person's own east", got)
	}

	// Clearing it falls back to the group rather than to nothing.
	if err := s.SetUserScope(ctx, acme, "u-1", nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Confinement(ctx, "u-1"); got["region"] != "west" {
		t.Fatalf("after clearing, confinement is %v, want the group's west", got)
	}
}

/*
Two groups disagreeing about one field is refused, not resolved.

Picking one silently is how somebody ends up reading a region nobody granted
them; picking the union is how adding a group widens a confinement that was
supposed to narrow. Neither is an answer, so there is no answer.
*/
func TestTwoGroupsDisagreeingAboutAFieldIsRefused(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	for name, region := range map[string]string{"west": "west", "east": "east"} {
		g := group(t, s, acme, name, map[string]string{"region": region})
		if err := s.AddToGroup(ctx, acme, g.ID, "u-1"); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.Confinement(ctx, "u-1")
	if !errors.Is(err, store.ErrScopeConflict) {
		t.Fatalf("got %v, want ErrScopeConflict", err)
	}
	// The message names both groups, because the fix is to change one of them
	// and the administrator has to know which two are fighting.
	if msg := err.Error(); !contains(msg, "west") || !contains(msg, "east") {
		t.Errorf("the error does not name both groups: %v", err)
	}
}

// Groups that agree are merged, which is the ordinary case for somebody in a
// regional group and a product-line group.
func TestGroupsThatConfineDifferentFieldsAreMerged(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	for name, scope := range map[string]map[string]string{
		"west":   {"region": "west"},
		"retail": {"line": "retail"},
	} {
		g := group(t, s, acme, name, scope)
		if err := s.AddToGroup(ctx, acme, g.ID, "u-1"); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Confinement(ctx, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if got["region"] != "west" || got["line"] != "retail" {
		t.Fatalf("confinement is %v, want both fields", got)
	}
}

/* -- grants ---------------------------------------------------------------- */

func TestAGrantIsRecordedAndReadBack(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	want := access.Grant{Report: "payroll", Kind: access.Group, Subject: "finance"}
	if err := s.Grant(ctx, acme, want); err != nil {
		t.Fatal(err)
	}
	// Twice, because a second administrator granting the same thing is not an
	// error.
	if err := s.Grant(ctx, acme, want); err != nil {
		t.Fatalf("granting twice: %v", err)
	}

	got, err := s.Grants(ctx, "acme", "finance")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("grants are %v, want exactly one %v", got, want)
	}

	if err := s.RevokeGrant(ctx, acme, want); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Grants(ctx, "acme", "finance"); len(got) != 0 {
		t.Fatalf("after revoking, grants are %v", got)
	}
}

// A grant that names nobody would look granted in a list and open nothing.
func TestAGrantMustNameAReportAndASubject(t *testing.T) {
	s := open(t)

	for _, g := range []access.Grant{
		{Report: "", Kind: access.User, Subject: "u-1"},
		{Report: "payroll", Kind: access.User, Subject: ""},
		{Report: "payroll", Kind: "everyone", Subject: "u-1"},
	} {
		if err := s.Grant(context.Background(), acme, g); err == nil {
			t.Errorf("accepted %+v", g)
		}
	}
}

/* -- tenancy --------------------------------------------------------------- */

/*
Everything here is scoped in the statement, not by the caller.

A grant is a permission, and a permission query that can be written without its
tenancy is one that eventually is. These assert the boundary from the outside:
another tenant's administrator sees none of it and can change none of it.
*/
func TestAnotherTenantSeesAndTouchesNoneOfIt(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	rival := who("rival", "ops")

	g := group(t, s, acme, "finance", map[string]string{"region": "west"})
	if err := s.Grant(ctx, acme, access.Grant{
		Report: "payroll", Kind: access.Group, Subject: "finance",
	}); err != nil {
		t.Fatal(err)
	}

	// Reads.
	if got, _ := s.Grants(ctx, "rival", "ops"); len(got) != 0 {
		t.Errorf("another tenant read %d grants", len(got))
	}
	if got, _ := s.Groups(ctx, "rival", "ops"); len(got) != 0 {
		t.Errorf("another tenant read %d groups", len(got))
	}

	// Writes, by naming an id they could only have guessed.
	if err := s.AddToGroup(ctx, rival, g.ID, "u-9"); err == nil {
		t.Error("another tenant added somebody to this group")
	}
	if err := s.SetGroupScope(ctx, rival, g.ID, map[string]string{"region": "all"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Groups(ctx, "acme", "finance"); got[0].Scope["region"] != "west" {
		t.Errorf("another tenant changed the scope to %v", got[0].Scope)
	}
	if err := s.DeleteGroup(ctx, rival, g.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Groups(ctx, "acme", "finance"); len(got) != 1 {
		t.Error("another tenant deleted this group")
	}

	// And revoking by naming the grant.
	if err := s.RevokeGrant(ctx, rival, access.Grant{
		Report: "payroll", Kind: access.Group, Subject: "finance",
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Grants(ctx, "acme", "finance"); len(got) != 1 {
		t.Error("another tenant revoked this grant")
	}
}

/*
Deleting a group takes its grants with it.

A grant to a group nobody can join is a permission that looks live in a list
and opens nothing — and a group recreated under the same name would silently
inherit it.
*/
func TestDeletingAGroupRemovesItsGrantsAndMembership(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	g := group(t, s, acme, "finance", nil)
	if err := s.AddToGroup(ctx, acme, g.ID, "u-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(ctx, acme, access.Grant{
		Report: "payroll", Kind: access.Group, Subject: "finance",
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteGroup(ctx, acme, g.ID); err != nil {
		t.Fatal(err)
	}

	if got, _ := s.Grants(ctx, "acme", "finance"); len(got) != 0 {
		t.Errorf("the group's grants outlived it: %v", got)
	}
	if got, _ := s.GroupsOf(ctx, "u-1"); len(got) != 0 {
		t.Errorf("a member still belongs to a deleted group: %v", got)
	}
	if got, _ := s.Confinement(ctx, "u-1"); got != nil {
		t.Errorf("a deleted group still confines: %v", got)
	}
}

func TestGroupsOfNamesWhatTheAccessDecisionNeeds(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	for _, name := range []string{"finance", "west"} {
		g := group(t, s, acme, name, nil)
		if err := s.AddToGroup(ctx, acme, g.ID, "u-1"); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.GroupsOf(ctx, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	// Sorted, so a grant list rendered from this is stable between reads.
	if len(got) != 2 || got[0] != "finance" || got[1] != "west" {
		t.Fatalf("groups are %v, want [finance west]", got)
	}

	if err := s.RemoveFromGroup(ctx, acme, // by id, resolved from the listing
		mustGroup(t, s, "west").ID, "u-1"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GroupsOf(ctx, "u-1"); len(got) != 1 || got[0] != "finance" {
		t.Fatalf("after removal, groups are %v", got)
	}
}

func mustGroup(t *testing.T, s *store.Store, name string) store.Group {
	t.Helper()
	all, err := s.Groups(context.Background(), "acme", "finance")
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range all {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("no group named %q", name)
	return store.Group{}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	}()
}
