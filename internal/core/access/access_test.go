package access_test

import (
	"testing"

	"github.com/gsoultan/cronos/internal/core/access"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Who may open which report.

The decision is small and every branch of it is somebody seeing data or not
seeing it, so the tests are the specification rather than a sample of it. Two
properties matter more than the rest:

  - a report nobody granted stays open, which is what makes an upgrade a no-op
  - grants for one report never decide another, which is the mistake that would
    turn one grant into a key for the whole project
*/

func person(role principal.Role, id string) principal.Principal {
	return principal.Principal{
		Subject: id, OrgID: "acme", ProjectID: "finance",
		ProjectRole: role, Member: true,
	}
}

func grant(report string, kind access.Kind, subject string) access.Grant {
	return access.Grant{Report: report, Kind: kind, Subject: subject}
}

/*
Nothing granted, nothing restricted.

The upgrade-safety property, and the one worth failing loudly on: if this ever
returns false, every report in every deployment stops opening at once.
*/
func TestAReportNobodyGrantedStaysOpenToTheProject(t *testing.T) {
	for _, role := range []principal.Role{
		principal.ProjectViewer, principal.ProjectEditor, principal.ProjectAdmin,
	} {
		if !access.Allowed(person(role, "u1"), nil, nil) {
			t.Errorf("%s was refused a report nobody restricted", role)
		}
	}
	if access.Restricted(nil) {
		t.Error("a report with no grants reports itself as restricted")
	}
}

func TestAGrantToThePersonOpensIt(t *testing.T) {
	grants := []access.Grant{grant("payroll", access.KindUser, "u1")}

	if !access.Allowed(person(principal.ProjectViewer, "u1"), grants, nil) {
		t.Error("the person named in the grant was refused")
	}
	if access.Allowed(person(principal.ProjectViewer, "u2"), grants, nil) {
		t.Error("somebody not named was allowed")
	}
	if !access.Restricted(grants) {
		t.Error("a granted report does not report itself as restricted")
	}
}

func TestAGrantToAGroupOpensItForItsMembers(t *testing.T) {
	grants := []access.Grant{grant("payroll", access.KindGroup, "finance")}

	if !access.Allowed(person(principal.ProjectViewer, "u1"), grants, []string{"finance", "ops"}) {
		t.Error("a member of the granted group was refused")
	}
	if access.Allowed(person(principal.ProjectViewer, "u2"), grants, []string{"ops"}) {
		t.Error("somebody in another group was allowed")
	}
	if access.Allowed(person(principal.ProjectViewer, "u3"), grants, nil) {
		t.Error("somebody in no group at all was allowed")
	}
}

/*
An editor is governed by grants, and that is the point of recording them in the
database rather than the definition.

An editor can rewrite the report's YAML. They cannot rewrite a grant, because
grants are an administrator's, so restricting a report from an editor is a
statement that actually holds.
*/
func TestAnEditorIsSubjectToGrants(t *testing.T) {
	grants := []access.Grant{grant("payroll", access.KindGroup, "finance")}

	if access.Allowed(person(principal.ProjectEditor, "u9"), grants, nil) {
		t.Error("an editor read a report nobody granted them")
	}
}

/*
An administrator is not, and that is not a hole.

They are who adds and removes grants. A deployment where an admin can be locked
out of a report has a recovery path that ends at a psql prompt — the same
reasoning docs/tenancy.md gives for an org owner entering any project.
*/
func TestAnAdministratorIsNotSubjectToGrants(t *testing.T) {
	grants := []access.Grant{grant("payroll", access.KindGroup, "finance")}

	if !access.Allowed(person(principal.ProjectAdmin, "u9"), grants, nil) {
		t.Error("a project admin was locked out of a report they administer")
	}

	// And an org administrator, who holds no project role at all.
	org := person("", "u8")
	org.OrgRole = principal.OrgAdmin
	if !access.Allowed(org, grants, nil) {
		t.Error("an org administrator was locked out")
	}
}

/*
Membership is asked first, and a grant cannot substitute for it.

Otherwise a grant naming somebody outside the project would admit them to it,
which is a tenancy boundary being crossed by a permission that was only ever
meant to narrow one.
*/
func TestAGrantDoesNotAdmitSomebodyWhoIsNotInTheProject(t *testing.T) {
	outsider := principal.Principal{Subject: "u1", OrgID: "rival", ProjectID: "ops"}

	if outsider.CanRead() {
		t.Fatal("the fixture is wrong: this principal has a role")
	}
	if access.Allowed(outsider, []access.Grant{grant("payroll", access.KindUser, "u1")}, nil) {
		t.Fatal("a grant admitted somebody with no membership")
	}
}

/*
One report's grants never decide another's.

For() is the only place that filtering happens, so this is the test that keeps
a caller from passing the whole project's grants and quietly granting
everything to anybody holding one.
*/
func TestGrantsAreScopedToOneReport(t *testing.T) {
	all := []access.Grant{
		grant("payroll", access.KindUser, "u1"),
		grant("billing", access.KindUser, "u2"),
	}

	if got := access.For("payroll", all); len(got) != 1 || got[0].Subject != "u1" {
		t.Fatalf("For returned %v", got)
	}
	// u2 holds a grant, on another report, and it opens nothing here.
	if access.Allowed(person(principal.ProjectViewer, "u2"), access.For("payroll", all), nil) {
		t.Error("a grant on another report opened this one")
	}
	// And a report with no grants of its own is still open.
	if !access.Allowed(person(principal.ProjectViewer, "u3"), access.For("other", all), nil) {
		t.Error("a report with no grants of its own was restricted by another's")
	}
}

// An empty subject matches nobody. A blank row is a configuration mistake, and
// reading it as a wildcard is how one bad insert opens a report to everyone.
func TestABlankSubjectMatchesNobody(t *testing.T) {
	for _, g := range []access.Grant{
		grant("payroll", access.KindUser, ""),
		grant("payroll", access.KindGroup, ""),
	} {
		if access.Allowed(person(principal.ProjectViewer, ""), []access.Grant{g}, []string{""}) {
			t.Errorf("a blank %s subject matched", g.Kind)
		}
	}
}

// An unknown kind matches nobody either, so a future kind added to the schema
// cannot open a report on a build that predates it.
func TestAnUnknownKindMatchesNobody(t *testing.T) {
	grants := []access.Grant{grant("payroll", access.Kind("everyone"), "u1")}

	if access.Allowed(person(principal.ProjectViewer, "u1"), grants, []string{"u1"}) {
		t.Error("a kind this build does not know opened a report")
	}
}
