package api_test

import (
	"context"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
One process serving several projects is a guarantee traded for an economy.

Before this, the blast radius of a bad definition, a runaway query or a leaked
signing key was one customer's project, because there was physically nothing
else in the process. Now that isolation is a property of the code: every
handler resolves its runtime from the caller's own principal, and a handler that
resolved the wrong one would put one customer's numbers on another customer's
screen with nothing in a log that looked unusual.

So these are the tests that matter more than any others in this package.
*/

var (
	acme   = principal.Principal{OrgID: "acme", ProjectID: "finance", ProjectRole: principal.ProjectEditor}
	globex = principal.Principal{OrgID: "globex", ProjectID: "ops", ProjectRole: principal.ProjectEditor}
)

func TestEachProjectResolvesToItsOwnRuntime(t *testing.T) {
	ours, theirs := &api.Project{}, &api.Project{}

	many := api.NewMany()
	many.Add("acme", "finance", ours)
	many.Add("globex", "ops", theirs)

	got, err := many.Project(context.Background(), acme)
	if err != nil {
		t.Fatal(err)
	}
	if got != ours {
		t.Fatal("acme got somebody else's runtime")
	}

	got, err = many.Project(context.Background(), globex)
	if err != nil {
		t.Fatal(err)
	}
	if got != theirs {
		t.Fatal("globex got somebody else's runtime")
	}
}

// A project this process was never told about is the same answer as one it
// serves and the caller does not belong to. Telling them apart would let
// somebody enumerate which customers a deployment holds.
func TestAProjectThisProcessDoesNotServeIsRefused(t *testing.T) {
	many := api.NewMany()
	many.Add("acme", "finance", &api.Project{})

	if _, err := many.Project(context.Background(), globex); err == nil {
		t.Fatal("a principal from a project nobody serves resolved a runtime")
	}
}

/*
The halves must not be swappable.

An organisation and a project joined by a separator either of them may contain
lets `acme` + `/finance` and `acme/fin` + `ance` land on the same key, which is
a cross-tenant read produced by string concatenation and nothing else.
*/
func TestTheTwoHalvesOfATenantCannotBeConfused(t *testing.T) {
	first, second := &api.Project{}, &api.Project{}

	many := api.NewMany()
	many.Add("acme", "finance", first)
	many.Add("acme/fin", "ance", second)

	got, err := many.Project(context.Background(),
		principal.Principal{OrgID: "acme", ProjectID: "finance"})
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatal("two tenants collided on one key")
	}
}

// A principal with no project resolves nothing. Empty strings would otherwise
// match a runtime registered with empty ones, which is a tenant nobody meant
// to create and everybody can reach.
func TestAPrincipalWithNoProjectResolvesNothing(t *testing.T) {
	many := api.NewMany()
	many.Add("", "", &api.Project{})

	if _, err := many.Project(context.Background(), principal.Principal{}); err == nil {
		t.Fatal("a principal naming no project resolved a runtime")
	}
}

/*
A single-project deployment checks too.

The narrowness of the deployment is not the check: a process holding one
project that served its definitions to a principal from another organisation is
the same leak as a multi-tenant one resolving the wrong runtime, and it is the
easier mistake to make because there is only ever one thing to return.
*/
func TestASingleProjectStillChecksWhoIsAsking(t *testing.T) {
	only := &api.Project{}
	one := &api.One{Org: "acme", ProjectID: "finance", Only: only}

	got, err := one.Project(context.Background(), acme)
	if err != nil || got != only {
		t.Fatalf("the owner was refused: %v", err)
	}

	if _, err := one.Project(context.Background(), globex); err == nil {
		t.Fatal("a principal from another organisation was served this project")
	}
	// Including one that names the right project in the wrong organisation,
	// which is the pair a check on only half of it would miss.
	sameProjectOtherOrg := principal.Principal{OrgID: "globex", ProjectID: "finance"}
	if _, err := one.Project(context.Background(), sameProjectOtherOrg); err == nil {
		t.Fatal("only the project name was checked")
	}
}

/*
The change this made to an embed token, which is worth stating on its own.

Before one process could serve several projects, the embed handler never read
the organisation and project out of a token: tenancy came from the signing key,
because one key meant one deployment and one deployment meant one project. Any
token signed with the right key opened any report on that server, whatever
project it claimed to be for.

That is no longer true, and it is a breaking change for a host that mints
tokens naming a project the server does not serve. It was already the wrong
thing when the claim was ignored — a token is a statement about where somebody
is acting, and ignoring half of it made the other half load-bearing by
accident.
*/
func TestATokenNamingAnotherProjectDoesNotOpenThisOne(t *testing.T) {
	one := &api.One{Org: "acme", ProjectID: "finance", Only: &api.Project{}}

	// Signed by the same key, for the same server, naming somewhere else.
	elsewhere := principal.Principal{
		OrgID: "acme", ProjectID: "marketing", ProjectRole: principal.ProjectViewer,
	}
	if _, err := one.Project(context.Background(), elsewhere); err == nil {
		t.Fatal("a token for another project of the same organisation was served")
	}
}

/*
A fresh deployment learns what it was called.

Found by driving a first run rather than by reasoning about it. A new install
serves whatever CRONOS_ORG and CRONOS_PROJECT default to, because nothing else
exists yet; then somebody opens /setup and names the organisation Acme
Logistics. Without this the account is created there, the process is still
serving default/default, and the first person signs in successfully and sees an
empty portal — refused by the very check that keeps tenants apart.
*/
func TestAFreshDeploymentAdoptsTheNameItIsGiven(t *testing.T) {
	only := &api.Project{}
	one := &api.One{Org: "default", ProjectID: "default", Only: only}

	named := principal.Principal{OrgID: "acme-logistics", ProjectID: "finance"}
	if _, err := one.Project(context.Background(), named); err == nil {
		t.Fatal("a project nobody has named yet resolved")
	}

	if !one.Adopt("acme-logistics", "finance") {
		t.Fatal("a deployment that has never been named refused to adopt one")
	}
	got, err := one.Project(context.Background(), named)
	if err != nil || got != only {
		t.Fatalf("after adopting: %v", err)
	}

	// And the placeholder it started with is no longer served, or the tenancy
	// would be two names for one runtime.
	was := principal.Principal{OrgID: "default", ProjectID: "default"}
	if _, err := one.Project(context.Background(), was); err == nil {
		t.Fatal("the deployment still answers to the name it was given at boot")
	}
}

/*
And is named once.

Adopting is the first run telling a deployment what it is, and the first run
happens once. A second call is either a bug or a second setup, and the endpoint
that calls this is closed by then — but the guarantee belongs here, where it can
be read next to the thing it protects.
*/
func TestADeploymentIsNamedOnlyOnce(t *testing.T) {
	one := &api.One{Org: "default", ProjectID: "default", Only: &api.Project{}}

	if !one.Adopt("acme", "finance") {
		t.Fatal("the first naming was refused")
	}
	if one.Adopt("globex", "ops") {
		t.Fatal("a second naming was accepted")
	}

	// The first name stands.
	if _, err := one.Project(context.Background(),
		principal.Principal{OrgID: "globex", ProjectID: "ops"}); err == nil {
		t.Fatal("the second name took effect")
	}
	if _, err := one.Project(context.Background(),
		principal.Principal{OrgID: "acme", ProjectID: "finance"}); err != nil {
		t.Fatalf("the first name was lost: %v", err)
	}
}

// Half a name is no name. Adopting an empty organisation would make the
// tenancy check match a principal that names nothing.
func TestAnEmptyNameIsNotAdopted(t *testing.T) {
	for _, c := range [][2]string{{"", "finance"}, {"acme", ""}, {"", ""}} {
		one := &api.One{Org: "default", ProjectID: "default", Only: &api.Project{}}
		if one.Adopt(c[0], c[1]) {
			t.Fatalf("adopted %q/%q", c[0], c[1])
		}
		if _, err := one.Project(context.Background(), principal.Principal{}); err == nil {
			t.Fatal("a principal naming nothing resolved a runtime")
		}
	}
}
