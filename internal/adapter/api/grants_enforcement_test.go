package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/gsoultan/cronos/internal/core/access"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
The gate between a grant in a table and a report on a screen.

core/access decides; this is the reading that feeds it, and the reading is
where the interesting failures are. Two of them:

  - a store that errors must not read as "no grants", which would open every
    restricted report in the deployment at the moment the database is slowest
  - a deployment with no store at all must open everything, because there is
    nowhere a grant could have been recorded — that is not a degraded mode, it
    is a file-backed deployment working correctly
*/

/* -- fakes ---------------------------------------------------------------- */

type grantStore struct {
	grants []access.Grant
	groups map[string][]string
	// failGrants and failGroups make each read fail on its own, because the
	// two are separate queries and only one of them may be broken.
	failGrants, failGroups bool
}

func (g grantStore) Grants(context.Context, string, string) ([]access.Grant, error) {
	if g.failGrants {
		return nil, errors.New("the database is not answering")
	}
	return g.grants, nil
}

func (g grantStore) GroupsOf(_ context.Context, id string) ([]string, error) {
	if g.failGroups {
		return nil, errors.New("the database is not answering")
	}
	return g.groups[id], nil
}

func viewerIn(id string) principal.Principal {
	return principal.Principal{
		Subject: id, OrgID: "acme", ProjectID: "finance",
		ProjectRole: principal.ProjectViewer, Member: true,
	}
}

// ask runs the gate the way both call sites do. An internal test, so the gate
// is exercised directly rather than through an exported wrapper that would
// exist only for tests.
func ask(t *testing.T, g Granting, pr principal.Principal, report string) (allowed, known bool) {
	t.Helper()
	return gate{grants: g, log: silent()}.may(context.Background(), pr, report)
}

func silent() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

/* -- the failure that matters ---------------------------------------------- */

/*
A store that cannot answer refuses, and never opens.

Reading an error as "no grants exist" would unlock every restricted report in
the deployment precisely when the database is least well — which is the one
time nobody is reading the logs.
*/
func TestAGrantStoreThatFailsRefusesRatherThanOpens(t *testing.T) {
	restricted := []access.Grant{{Report: "payroll", Kind: access.KindGroup, Subject: "finance"}}

	for _, c := range []struct {
		name  string
		store grantStore
	}{
		{"grants unreadable", grantStore{failGrants: true}},
		{"membership unreadable", grantStore{grants: restricted, failGroups: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			allowed, known := ask(t, c.store, viewerIn("u-1"), "payroll")

			if known {
				t.Error("a failed read reported a trustworthy answer")
			}
			if allowed {
				t.Error("a failed read opened the report")
			}
		})
	}
}

// No store at all opens everything. A file-backed deployment has nowhere to
// record a grant, so there is nothing anybody could have restricted.
func TestADeploymentWithNoStoreOpensEverything(t *testing.T) {
	allowed, known := ask(t, nil, viewerIn("u-1"), "payroll")

	if !known || !allowed {
		t.Fatalf("allowed=%v known=%v, want both true", allowed, known)
	}
}

/* -- the ordinary decisions ------------------------------------------------ */

func TestAnUngrantedReportOpensForTheProject(t *testing.T) {
	store := grantStore{grants: []access.Grant{
		// Another report is restricted; this one is not.
		{Report: "payroll", Kind: access.KindGroup, Subject: "finance"},
	}}

	if allowed, known := ask(t, store, viewerIn("u-1"), "billing"); !allowed || !known {
		t.Fatalf("an ungranted report was closed: allowed=%v known=%v", allowed, known)
	}
}

func TestAGrantedReportOpensOnlyForThoseNamed(t *testing.T) {
	store := grantStore{
		grants: []access.Grant{{Report: "payroll", Kind: access.KindGroup, Subject: "finance"}},
		groups: map[string][]string{"u-1": {"finance"}, "u-2": {"ops"}},
	}

	if allowed, _ := ask(t, store, viewerIn("u-1"), "payroll"); !allowed {
		t.Error("somebody in the granted group was refused")
	}
	if allowed, _ := ask(t, store, viewerIn("u-2"), "payroll"); allowed {
		t.Error("somebody in another group was allowed")
	}
	if allowed, _ := ask(t, store, viewerIn("u-3"), "payroll"); allowed {
		t.Error("somebody in no group was allowed")
	}
}

/*
An administrator opens it whatever the grants say.

They are who adds and removes grants, so a deployment where an admin can be
locked out of a report has a recovery path that ends at a psql prompt.
*/
func TestAnAdministratorIsNotGatedByGrants(t *testing.T) {
	store := grantStore{
		grants: []access.Grant{{Report: "payroll", Kind: access.KindGroup, Subject: "finance"}},
	}
	admin := viewerIn("u-9")
	admin.ProjectRole = principal.ProjectAdmin

	if allowed, _ := ask(t, store, admin, "payroll"); !allowed {
		t.Error("a project admin was locked out of a report they administer")
	}
}

/* -- the bulk path --------------------------------------------------------- */

/*
The catalogue asks once for the whole list.

A permission check that costs a pair of queries per row is how a page becomes
slow, and a slow page is how somebody ends up caching an access decision.
*/
func TestTheCatalogueDecidesTheWholeListInOnePass(t *testing.T) {
	store := grantStore{
		grants: []access.Grant{
			{Report: "payroll", Kind: access.KindGroup, Subject: "finance"},
			{Report: "board", Kind: access.KindUser, Subject: "u-9"},
		},
		groups: map[string][]string{"u-1": {"finance"}},
	}

	open, known := gate{grants: store, log: silent()}.visible(context.Background(),
		viewerIn("u-1"), []string{"payroll", "board", "billing"})
	if !known {
		t.Fatal("the catalogue could not decide")
	}

	for name, want := range map[string]bool{
		"payroll": true,  // granted to their group
		"board":   false, // granted to somebody else
		"billing": true,  // granted to nobody, so open
	} {
		if open[name] != want {
			t.Errorf("%s: visible=%v, want %v", name, open[name], want)
		}
	}
}

// And a failed read hides everything rather than showing everything.
func TestAFailedReadHidesTheWholeCatalogue(t *testing.T) {
	_, known := gate{grants: grantStore{failGrants: true}, log: silent()}.
		visible(context.Background(), viewerIn("u-1"), []string{"payroll"})

	if known {
		t.Error("a failed read reported a trustworthy catalogue")
	}
}

/* -- the outage the failover check holds ----------------------------------- */

/*
A store that has gone away must not stop an editor reading a report.

docs/deploying.md promises that reports keep rendering through an outage of
cronos's own database — definitions are in memory and the warehouse is somebody
else's server. scripts/live-failover.sh holds it, and this is the unit-level
guard for the way it was broken: refusing everybody whose confinement could not
be read, including the roles a confinement never applies to.

Only a viewer can be confined. Asking the question of anybody else spends a
query to discard the answer, and refusing them over it turns a store outage
into an outage of the whole portal — at the moment somebody is trying to read a
report to find out what is wrong.
*/
func TestOnlyAViewerIsRefusedWhenAConfinementCannotBeRead(t *testing.T) {
	for _, c := range []struct {
		role       principal.Role
		confinable bool
	}{
		{principal.ProjectViewer, true},
		{principal.ProjectEditor, false},
		{principal.ProjectAdmin, false},
	} {
		pr := viewerIn("u-1")
		pr.ProjectRole = c.role

		if got := pr.Confinable(); got != c.confinable {
			t.Errorf("%s: Confinable = %v, want %v", c.role, got, c.confinable)
		}
	}

	// And an org administrator, who holds no project role at all, is exempt —
	// which is who is most likely to be reading a report during an incident.
	org := viewerIn("u-8")
	org.ProjectRole = principal.None
	org.OrgRole = principal.OrgAdmin
	if org.Confinable() {
		t.Error("an org administrator was treated as confinable")
	}
}
