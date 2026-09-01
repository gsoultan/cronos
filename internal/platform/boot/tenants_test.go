package boot

import (
	"context"
	"errors"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/app/send"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Publishing survives a first run naming the deployment.

A fresh install serves whatever CRONOS_ORG defaults to, because nothing else
exists yet. Somebody opens /setup, calls their organisation Acme, and their
account is created there — api.One adopts the name so their reads work.

Three helpers used to keep their own map of "which tenant is this", built at
boot, still holding one entry under default/default. Reads worked and every
write did not: publish, send and share all answered "no such project here". A
deployment set up through the browser could look at its reports and change
nothing, which is most of a product being unusable behind a screen that says
Welcome.

The reason it survived so long is that every test in this repository builds a
process already knowing its tenancy, and every fixture agrees with itself. The
disagreement only exists between boot time and the moment somebody types a name
in a browser — so this test does that, in that order, which is the only order in
which it is a bug.
*/
func TestWritesWorkAfterAFirstRunNamesTheDeployment(t *testing.T) {
	only := &api.Project{}
	one := &api.One{Org: "default", ProjectID: "default", Only: only}

	// Boot: the process serves default/default, and its map is keyed by the
	// runtime rather than by that name.
	pub := publishingFor{projects: one, byProject: map[*api.Project]*publish.Service{
		only: nil, // resolution is what is under test, not the service.
	}}
	sender := sendPerProject{projects: one, byProject: map[*api.Project]*send.Service{
		only: nil,
	}}

	// The first run, which is the only thing that can do this.
	if !one.Adopt("acme", "finance") {
		t.Fatal("a fresh deployment refused to adopt a name")
	}

	// Somebody who signed in after the first run.
	pr := principal.Principal{
		Subject: "usr_ada", OrgID: "acme", ProjectID: "finance",
		ProjectRole: principal.ProjectAdmin,
	}

	if _, err := pub.of(context.Background(), pr); err != nil {
		t.Errorf("publishing after a first run: %v", err)
	}
	/*
	   Send takes the same path, and the assertion is that it is not refused on
	   tenancy — not that it succeeds. The request here is empty, so the service
	   below rightly calls it invalid; what must not happen is ErrForbidden,
	   which is the only thing this resolution can say.

	   The two being separate sentinels is why this can be asserted at all. They
	   were the same until a moment ago, and "sending after a first run" was
	   indistinguishable from "that request was malformed".
	*/
	if _, err := sender.Send(context.Background(), send.Request{}, pr); errors.Is(err, send.ErrForbidden) {
		t.Errorf("sending after a first run: %v", err)
	}
}

/*
And the boot-time tenancy stops working, which is the same fact from the other
side.

Keying by pointer identity means a principal is checked against what the process
serves now, not against what it served at startup. Somebody still carrying the
old default/default — a token minted in the seconds before setup finished — is
refused. That is correct: the deployment is Acme's now.

Without this half, a map that simply held both names would pass the test above
and would be a tenancy check that never says no.
*/
func TestTheTenancyFromBeforeTheFirstRunIsRefused(t *testing.T) {
	only := &api.Project{}
	one := &api.One{Org: "default", ProjectID: "default", Only: only}
	pub := publishingFor{projects: one, byProject: map[*api.Project]*publish.Service{only: nil}}

	one.Adopt("acme", "finance")

	stale := principal.Principal{
		Subject: "usr_ada", OrgID: "default", ProjectID: "default",
		ProjectRole: principal.ProjectAdmin,
	}
	_, err := pub.of(context.Background(), stale)
	if !errors.Is(err, publish.ErrForbidden) {
		t.Fatalf("a token from before the first run could still publish: %v", err)
	}
}

// A principal from an organisation this process has never served is refused
// whether or not anybody has adopted anything. The narrowness of a
// single-project deployment is not the check.
func TestAnotherOrganisationCannotPublishHere(t *testing.T) {
	only := &api.Project{}
	one := &api.One{Org: "acme", ProjectID: "finance", Only: only}
	pub := publishingFor{projects: one, byProject: map[*api.Project]*publish.Service{only: nil}}

	_, err := pub.of(context.Background(), principal.Principal{
		Subject: "usr_eve", OrgID: "rival", ProjectID: "finance",
		ProjectRole: principal.ProjectAdmin,
	})
	if !errors.Is(err, publish.ErrForbidden) {
		t.Fatalf("another organisation published here: %v", err)
	}
}

/*
A deployment told what it serves cannot be renamed by an HTTP request.

Adopt exists for the case where nothing was told to the process. If a first run
could rename a configured deployment, /setup would be a way to move somebody
else's project into your organisation — and /setup answers before there is
anybody to be an administrator.
*/
func TestAConfiguredDeploymentIsNotRenamedTwice(t *testing.T) {
	one := &api.One{Org: "default", ProjectID: "default", Only: &api.Project{}}

	if !one.Adopt("acme", "finance") {
		t.Fatal("the first adoption was refused")
	}
	if one.Adopt("rival", "finance") {
		t.Fatal("a second call renamed the deployment")
	}

	pr := principal.Principal{OrgID: "rival", ProjectID: "finance"}
	if _, err := one.Project(context.Background(), pr); err == nil {
		t.Fatal("the second name took effect")
	}
}
