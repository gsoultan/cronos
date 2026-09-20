package sql_test

import (
	"context"
	"errors"
	"testing"
	"time"

	store "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/access"
	"github.com/gsoultan/cronos/internal/core/identity"
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
	name string, scope map[string]string) access.Group {

	t.Helper()
	g, err := s.CreateGroup(context.Background(), pr, name, scope)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	return g
}

/*
joins makes sure an account exists in acme/finance and returns its id.

AddToGroup verifies the target belongs to the caller's project now — an id that
is merely well-formed is refused, which is the whole point of the check — so
membership tests need a real account. Idempotent, because a person can be in
two groups and the second call would otherwise fail on the unique email.
*/
func joins(t *testing.T, s *store.Store, id string) string {
	t.Helper()

	err := s.CreateUser(context.Background(), identity.User{
		ID: id, Email: id + "@acme.example",
		Org: "acme", Project: "finance", Role: "viewer",
	}, "n0t-a-real-password")
	if err != nil && !errors.Is(err, identity.ErrExists) {
		t.Fatalf("creating %s: %v", id, err)
	}
	return id
}

// reader is somebody in acme/finance with the least role there is, so what
// opens a report for them is a grant and nothing else.
func reader(id string) principal.Principal {
	return principal.Principal{
		Subject: id, OrgID: "acme", ProjectID: "finance",
		ProjectRole: principal.ProjectViewer, Member: true,
	}
}

/* -- confinement ----------------------------------------------------------- */

func TestSomebodyInNoGroupIsConfinedByNothing(t *testing.T) {
	s := open(t)

	got, err := s.Confinement(context.Background(), "acme", "finance", "u-1")
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
	if err := s.AddToGroup(ctx, acme, g.ID, joins(t, s, "u-1")); err != nil {
		t.Fatal(err)
	}

	got, err := s.Confinement(ctx, "acme", "finance", "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if got["region"] != "west" {
		t.Fatalf("confinement is %v, want region=west", got)
	}
	// And somebody who is not in it is not confined by it.
	if other, _ := s.Confinement(ctx, "acme", "finance", "u-2"); other != nil {
		t.Errorf("a non-member was confined: %v", other)
	}
}

// A person's own scope overrides their groups' rather than merging with it.
// Two sources for one answer is how somebody reads a region nobody granted.
func TestAPersonsOwnScopeOverridesTheirGroups(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	g := group(t, s, acme, "west", map[string]string{"region": "west"})
	if err := s.AddToGroup(ctx, acme, g.ID, joins(t, s, "u-1")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserScope(ctx, acme, "u-1", map[string]string{"region": "east"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Confinement(ctx, "acme", "finance", "u-1")
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
	if got, _ = s.Confinement(ctx, "acme", "finance", "u-1"); got["region"] != "west" {
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
		if err := s.AddToGroup(ctx, acme, g.ID, joins(t, s, "u-1")); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.Confinement(ctx, "acme", "finance", "u-1")
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
		if err := s.AddToGroup(ctx, acme, g.ID, joins(t, s, "u-1")); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Confinement(ctx, "acme", "finance", "u-1")
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

	// The group exists first, because a grant to one that does not is refused
	// — see TestAGrantToSomebodyWhoIsNotHereIsRefused for why that matters.
	group(t, s, acme, "finance", nil)
	want := access.Grant{Report: "payroll", Kind: access.KindGroup, Subject: "finance"}
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

/*
A grant to somebody who is not here is refused, and that is not pedantry.

The first grant on a report is what makes it restricted. So a mistyped account
id does not grant nothing — it takes the report away from everybody who could
read it a moment ago and gives it to nobody, with a grant in the list and a
padlock on the page to say it worked. Nothing in the interface can tell that
state from a deliberate one.
*/
func TestAGrantToSomebodyWhoIsNotHereIsRefused(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	for _, c := range []struct {
		what string
		g    access.Grant
	}{
		{"an account id nobody holds",
			access.Grant{Report: "payroll", Kind: access.KindUser, Subject: "usr_typo"}},
		{"a group name nobody created",
			access.Grant{Report: "payroll", Kind: access.KindGroup, Subject: "finanace"}},
	} {
		err := s.Grant(ctx, acme, c.g)
		if !errors.Is(err, access.ErrNoSuchSubject) {
			t.Errorf("%s: got %v, want ErrNoSuchSubject", c.what, err)
		}
	}

	// And nothing was written, so the report is still open to the project.
	if got, _ := s.Grants(ctx, "acme", "finance"); len(got) != 0 {
		t.Fatalf("a refused grant left %v behind — the report is now restricted to nobody", got)
	}
}

/*
Somebody in another tenant is not here either.

An account id is not guessable, but it is copyable: two administrators in one
deployment share a Slack channel. The row would be inert — access.Allowed asks
for membership first — and it would still restrict the report.
*/
func TestAGrantToAnAccountInAnotherProjectIsRefused(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	if err := s.CreateUser(ctx, identity.User{
		ID: "usr_elsewhere", Email: "sam@rival.example",
		Org: "rival", Project: "ops", Role: "admin",
	}, "n0t-a-real-password"); err != nil {
		t.Fatal(err)
	}

	err := s.Grant(ctx, acme, access.Grant{
		Report: "payroll", Kind: access.KindUser, Subject: "usr_elsewhere",
	})
	if !errors.Is(err, access.ErrNoSuchSubject) {
		t.Fatalf("got %v, want ErrNoSuchSubject", err)
	}
}

/*
Revoking is not checked the same way, on purpose.

A person removed from the project leaves their grants behind. If revoking
needed them to still be here, those rows could only be removed with a database
prompt — and every one of them keeps a report restricted.
*/
func TestAGrantCanBeRevokedAfterThePersonIsGone(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	g := access.Grant{Report: "payroll", Kind: access.KindUser, Subject: joins(t, s, "u-1")}
	if err := s.Grant(ctx, acme, g); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeGrant(ctx, acme, g); err != nil {
		t.Fatalf("a grant could not be revoked: %v", err)
	}
	if got, _ := s.Grants(ctx, "acme", "finance"); len(got) != 0 {
		t.Fatalf("after revoking, grants are %v", got)
	}

	// The proof that revoking asks nothing about the subject: a name this
	// project has never held is accepted and removes nothing. However somebody
	// left — an account deleted, a person moved to another project, a group
	// dropped — the row they are named in has to be removable.
	for _, gone := range []access.Grant{
		{Report: "payroll", Kind: access.KindUser, Subject: "usr_long_gone"},
		{Report: "payroll", Kind: access.KindGroup, Subject: "a-group-that-was"},
	} {
		if err := s.RevokeGrant(ctx, acme, gone); err != nil {
			t.Errorf("revoking a grant naming %q: %v", gone.Subject, err)
		}
	}
}

// A grant that names nobody would look granted in a list and open nothing.
func TestAGrantMustNameAReportAndASubject(t *testing.T) {
	s := open(t)

	for _, g := range []access.Grant{
		{Report: "", Kind: access.KindUser, Subject: "u-1"},
		{Report: "payroll", Kind: access.KindUser, Subject: ""},
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
		Report: "payroll", Kind: access.KindGroup, Subject: "finance",
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
		Report: "payroll", Kind: access.KindGroup, Subject: "finance",
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
	if err := s.AddToGroup(ctx, acme, g.ID, joins(t, s, "u-1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(ctx, acme, access.Grant{
		Report: "payroll", Kind: access.KindGroup, Subject: "finance",
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteGroup(ctx, acme, g.ID); err != nil {
		t.Fatal(err)
	}

	if got, _ := s.Grants(ctx, "acme", "finance"); len(got) != 0 {
		t.Errorf("the group's grants outlived it: %v", got)
	}
	if got, _ := s.GroupsOf(ctx, "acme", "finance", "u-1"); len(got) != 0 {
		t.Errorf("a member still belongs to a deleted group: %v", got)
	}
	if got, _ := s.Confinement(ctx, "acme", "finance", "u-1"); got != nil {
		t.Errorf("a deleted group still confines: %v", got)
	}
}

func TestGroupsOfNamesWhatTheAccessDecisionNeeds(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	for _, name := range []string{"finance", "west"} {
		g := group(t, s, acme, name, nil)
		if err := s.AddToGroup(ctx, acme, g.ID, joins(t, s, "u-1")); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.GroupsOf(ctx, "acme", "finance", "u-1")
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
	if got, _ = s.GroupsOf(ctx, "acme", "finance", "u-1"); len(got) != 1 || got[0] != "finance" {
		t.Fatalf("after removal, groups are %v", got)
	}
}

func mustGroup(t *testing.T, s *store.Store, name string) access.Group {
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
	return access.Group{}
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

/*
A group in one project never answers for a person in another.

The gap this closes: cronos_group_members carries no tenancy of its own, and a
grant names a group by *name* — unique per project only. So a membership row
pointing at a same-named group in another project satisfied a grant here, and
the two ways such a row came to exist were an administrator adding a foreign
account to their own group, and an account being moved between projects with
its rows left behind. Both are closed below.
*/
func TestAGroupInAnotherProjectDoesNotAnswerHere(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	rival := who("rival", "ops")

	// The same name in two projects, which the unique index permits.
	mine := group(t, s, acme, "finance", map[string]string{"region": "west"})
	if _, err := s.CreateGroup(ctx, rival, "finance", map[string]string{"region": "east"}); err != nil {
		t.Fatal(err)
	}

	u := joins(t, s, "u-1") // an account in acme/finance
	if err := s.AddToGroup(ctx, acme, mine.ID, u); err != nil {
		t.Fatal(err)
	}

	// The other project's administrator tries to bind this account to a group
	// of theirs. Refused: the group is theirs, the person is not.
	foreign, err := s.Groups(ctx, "rival", "ops")
	if err != nil || len(foreign) != 1 {
		t.Fatalf("fixture: rival has %d groups, %v", len(foreign), err)
	}
	if err := s.AddToGroup(ctx, rival, foreign[0].ID, u); err == nil {
		t.Error("an administrator added somebody from another project to their group")
	}

	// The reads are scoped too, which tenancy_internal_test.go asserts against
	// a row planted directly — the write guard above and the read predicate are
	// two defences and either alone would leave the other untested.
	groups, err := s.GroupsOf(ctx, "acme", "finance", u)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0] != "finance" {
		t.Fatalf("groups are %v — the other project's membership answered here", groups)
	}

	// The confinement likewise: acme's west, never rival's east.
	scope, err := s.Confinement(ctx, "acme", "finance", u)
	if err != nil {
		t.Fatalf("the foreign group's scope conflicted: %v", err)
	}
	if scope["region"] != "west" {
		t.Errorf("confinement is %v, want acme's west", scope)
	}
}

// Moving somebody between projects takes their membership with them, so a
// stale row cannot arrive with them.
func TestMovingSomebodyClearsTheirGroupMembership(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	g := group(t, s, acme, "finance", nil)
	u := joins(t, s, "u-1")
	if err := s.AddToGroup(ctx, acme, g.ID, u); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GroupsOf(ctx, "acme", "finance", u); len(got) != 1 {
		t.Fatalf("membership was not recorded: %v", got)
	}

	if err := s.MovePerson(ctx, u, "rival", "ops", "viewer"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GroupsOf(ctx, "acme", "finance", u); len(got) != 0 {
		t.Errorf("membership survived the move: %v", got)
	}
}

/*
A personal confinement belongs to the project the person is in.

cronos_user_scopes is keyed by account alone, and SetUserScope read the
principal only for set_by — so an administrator in one project could change, and
more to the point delete, the personal scope of an account in another. Deleting
one *widens* what somebody sees: a person's own scope overrides their groups',
so removing it drops them back to their groups' or to nothing.

Nothing routes to SetUserScope today. The test exists because the gap was in
the method, and the route is the part somebody adds later without re-reading it.
*/
func TestOnlyThePersonsOwnProjectMaySetTheirScope(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	rival := who("rival", "ops")

	u := joins(t, s, "u-1") // an account in acme/finance
	if err := s.SetUserScope(ctx, acme, u, map[string]string{"region": "west"}); err != nil {
		t.Fatalf("their own administrator could not set it: %v", err)
	}

	// Another project's administrator may not change it...
	if err := s.SetUserScope(ctx, rival, u, map[string]string{"region": "east"}); err == nil {
		t.Error("an administrator in another project rewrote somebody's confinement")
	}
	// ...and may not remove it, which is the direction that widens.
	if err := s.SetUserScope(ctx, rival, u, nil); err == nil {
		t.Error("an administrator in another project deleted somebody's confinement")
	}

	scope, err := s.Confinement(ctx, "acme", "finance", u)
	if err != nil {
		t.Fatal(err)
	}
	if scope["region"] != "west" {
		t.Errorf("confinement is %v, want the west it was set to", scope)
	}
}

/*
Assigning a report to somebody who has been invited and has not arrived.

A report is restricted by its first grant, so "invite Sam and give them the
receivables summary" had to be done in that order and then remembered — and
the way it was remembered was somebody coming back days later, if at all.
Granting up front was refused, because there is no account to name yet.

The promise has to become a permission on its own. Nobody goes back.
*/
func TestAnInvitedPersonCanBeAssignedBeforeTheyArrive(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	secret := invite(t, s, "sam@acme.example", time.Hour)
	g := access.Grant{Report: "payroll", Kind: access.KindInvited, Subject: "sam@acme.example"}
	if err := s.Grant(ctx, acme, g); err != nil {
		t.Fatalf("granting to an invited address: %v", err)
	}

	// It restricts the report now, which is what was asked for.
	got, err := s.Grants(ctx, "acme", "finance")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != g {
		t.Fatalf("grants are %v, want the invited one", got)
	}
	// And opens nothing, because there is nobody holding it.
	if access.Allowed(reader("sam@acme.example"), got, nil) {
		t.Error("an invited grant opened a report for a session carrying that address")
	}

	// Accepting is where the promise becomes a permission.
	user, err := s.Accept(ctx, secret, "a-password-they-chose")
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.Grants(ctx, "acme", "finance")
	if err != nil {
		t.Fatal(err)
	}
	want := access.Grant{Report: "payroll", Kind: access.KindUser, Subject: user.ID}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("after accepting, grants are %v, want %v", got, want)
	}
	if !access.Allowed(reader(user.ID), got, nil) {
		t.Error("the person who accepted cannot open the report they were invited to read")
	}
}

// An address nobody invited is refused like any other subject that is not
// here — otherwise this kind would be the hole the other two just closed.
func TestAnAddressNobodyInvitedIsRefused(t *testing.T) {
	s := open(t)
	ctx := context.Background()

	err := s.Grant(ctx, acme, access.Grant{
		Report: "payroll", Kind: access.KindInvited, Subject: "stranger@example.com",
	})
	if !errors.Is(err, access.ErrNoSuchSubject) {
		t.Fatalf("got %v, want ErrNoSuchSubject", err)
	}
}

/*
An invitation that has expired is somebody who cannot arrive.

Accepting is what rewrites the grant, and an expired invitation can never be
accepted — so the row would sit there restricting the report for a person who
will never hold it.
*/
func TestAnExpiredInvitationCannotBeAssigned(t *testing.T) {
	s := open(t)

	invite(t, s, "late@acme.example", -time.Hour)

	err := s.Grant(context.Background(), acme, access.Grant{
		Report: "payroll", Kind: access.KindInvited, Subject: "late@acme.example",
	})
	if !errors.Is(err, access.ErrNoSuchSubject) {
		t.Fatalf("got %v, want ErrNoSuchSubject", err)
	}
}

// Another tenant's invitation is not one here, however well-known the address.
func TestAnInvitationInAnotherProjectIsRefused(t *testing.T) {
	s := open(t)

	invite(t, s, "sam@acme.example", time.Hour)

	err := s.Grant(context.Background(), who("rival", "ops"), access.Grant{
		Report: "payroll", Kind: access.KindInvited, Subject: "sam@acme.example",
	})
	if !errors.Is(err, access.ErrNoSuchSubject) {
		t.Fatalf("got %v, want ErrNoSuchSubject", err)
	}
}
