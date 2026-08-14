package boot

import (
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
A schedule runs as somebody, in somewhere.

One process serving several projects means several schedulers, and the failure
if the owner is wrong is not an error anybody sees: the burst reads the wrong
project's warehouse and mails the result to the wrong company's customers. It
succeeds, it is recorded as a success, and the first person to find out is the
recipient.

The owner used to be built from configuration, which was correct only because a
process served one project. These are the tests that it is built from the
project the scheduler belongs to.
*/

func TestASchedulesOwnerIsItsOwnProject(t *testing.T) {
	acme := owner{org: "acme", project: "finance"}
	globex := owner{org: "globex", project: "ops"}

	monthly := definition.Schedule{Name: "monthly-statements"}

	mine := acme.Owner(monthly)
	if mine.OrgID != "acme" || mine.ProjectID != "finance" {
		t.Fatalf("acme's schedule runs as %s/%s", mine.OrgID, mine.ProjectID)
	}

	// The same schedule name in another project. Nothing about the name
	// decides which warehouse it reads, so a shared owner would be invisible
	// right up until the wrong customers received something.
	theirs := globex.Owner(monthly)
	if theirs.OrgID != "globex" || theirs.ProjectID != "ops" {
		t.Fatalf("globex's schedule runs as %s/%s", theirs.OrgID, theirs.ProjectID)
	}
}

/*
And it runs as a member.

docs/tenancy.md is explicit that row scope applies to holders of an embed token
and that a schedule's owner is exempt, because a burst is the project acting on
its own data. A schedule that ran as an end customer would produce the
fail-closed predicate and mail everybody a page of em dashes.
*/
func TestASchedulesOwnerIsAProjectMember(t *testing.T) {
	who := owner{org: "acme", project: "finance"}.Owner(definition.Schedule{Name: "monthly"})

	if !who.Member {
		t.Fatal("a schedule runs as an end customer, so row scope applies to it")
	}
	if who.ProjectRole != principal.ProjectEditor {
		t.Fatalf("role is %q", who.ProjectRole)
	}
	if len(who.Scope) != 0 {
		t.Fatalf("a schedule carries a row scope: %v", who.Scope)
	}
}

// Named after the schedule, so a run record and an audit entry say which one
// acted rather than naming a person who was asleep.
func TestASchedulesOwnerIsNamedAfterIt(t *testing.T) {
	who := owner{org: "acme", project: "finance"}.Owner(definition.Schedule{Name: "monthly"})
	if who.Subject != "schedule:monthly" {
		t.Fatalf("subject is %q", who.Subject)
	}
}

/*
Two projects, two schedulers, and neither can see the other's schedules.

The armed set comes from each project's own repository. A single scheduler over
a shared source would fire every project's schedules under whichever owner it
was built with, which is the failure above at the scale of every schedule at
once.
*/
func TestEachProjectsSchedulerSeesOnlyItsOwn(t *testing.T) {
	dir := definitionsIn(t, scheduleDoc("acme-monthly"))
	other := definitionsIn(t, scheduleDoc("globex-weekly"))

	ours := loadRepo(t, dir)
	theirs := loadRepo(t, other)

	if names := scheduleNames(ours.Schedules()); len(names) != 1 || names[0] != "acme-monthly" {
		t.Fatalf("acme's repository holds %v", names)
	}
	if names := scheduleNames(theirs.Schedules()); len(names) != 1 || names[0] != "globex-weekly" {
		t.Fatalf("globex's repository holds %v", names)
	}
}

func scheduleNames(in []definition.Schedule) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, s.Name)
	}
	return out
}

// scheduleDoc is a schedule and the report and dataset it needs to load.
func scheduleDoc(name string) string {
	return `apiVersion: cronos.dev/v1
kind: Schedule
metadata:
  name: ` + name + `
spec:
  report: statement
  output: pdf
  cron: "0 6 1 * *"
  timezone: Europe/Berlin
  deliver:
    - via: email
      to: finance@example.com
`
}

/*
A scheduled run is filed under the tenancy the deployment has now.

The owner was captured from configuration when the scheduler was built, and a
deployment can be named once after that: it starts as default/default and
somebody opens /setup and calls it Acme. Every scheduled run then went on
recording itself under default/default, where the people who own it cannot see
it — the run history is read as the caller's tenant, and the caller is in the
adopted one.

Quiet, unlike its sibling. publishingFor had the same fault and failed loudly
with "no such project here"; this succeeded, and filed the record where nobody
would look. It was found by trying to resume a partly-delivered burst and being
told there was no such run.
*/
func TestAScheduledRunFollowsTheDeploymentsName(t *testing.T) {
	serving := func() (string, string) { return "acme", "finance" }

	who := owner{org: "default", project: "default", serving: serving}.
		Owner(definition.Schedule{Name: "monthly"})

	if who.OrgID != "acme" || who.ProjectID != "finance" {
		t.Fatalf("a scheduled run acts as %s/%s, so its record is filed where nobody looks",
			who.OrgID, who.ProjectID)
	}
}

// And a deployment that named itself in configuration is unaffected: there is
// nothing to adopt, and asking would be a slower way to the same answer.
func TestAConfiguredDeploymentKeepsItsOwnName(t *testing.T) {
	who := owner{org: "acme", project: "finance"}.Owner(definition.Schedule{Name: "monthly"})
	if who.OrgID != "acme" || who.ProjectID != "finance" {
		t.Fatalf("owner = %s/%s", who.OrgID, who.ProjectID)
	}
}
