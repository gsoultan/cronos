package api

import (
	"context"
	"log/slog"

	"github.com/gsoultan/cronos/internal/core/access"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Enforcing who may open which report.

The decision itself is access.Allowed, in core, with no database and no request
in it. What lives here is the reading — a project's grants and the caller's
groups — and the one rule about what to do when that reading fails.

Written once and called from both the catalogue and the report read, because
those are the two places the answer is needed and two copies of an access check
is one copy that gets a fix.
*/

// Granting is what the access decision needs from a store.
//
// Absent on a file-backed deployment, where there is nowhere to record a grant
// and nobody is restricted — the behaviour before this existed.
type Granting interface {
	Grants(ctx context.Context, org, project string) ([]access.Grant, error)
	// GroupsOf takes the project as well as the person. A grant names a group
	// by name and names are unique per project only, so a membership resolved
	// without one can satisfy a grant it was never meant to.
	GroupsOf(ctx context.Context, org, project, userID string) ([]string, error)
}

/*
gate answers "may this principal open this report", and says nothing about how.

A nil Granting means no grants exist anywhere, so everything is open. That is
the file-backed deployment and it is not a degraded mode: there is no store, so
there is nothing anybody could have restricted.

A Granting that errors is different and is refused. Reading a failure as "no
grants" would open every restricted report in the deployment at the moment the
database is slowest, which is the one time nobody is watching the logs.
*/
type gate struct {
	grants Granting
	log    *slog.Logger
}

/*
bypasses reports whether the answer is already known without asking the store.

An administrator is not subject to grants — access.Allowed says so on its own —
so reading them for one spends a round trip to discard the result. Checked
before the query rather than after, and that is not only about the round trip:
docs/deploying.md promises reports keep rendering through an outage of cronos's
own database, and a grant read that refuses on failure would break exactly the
person most likely to be reading a report while they fix it.

The refusal still stands for everybody else. A viewer during a store outage is
refused rather than served a report that might be restricted, which is an
outage for viewers and the safe direction to fail in.
*/
func (g gate) bypasses(pr principal.Principal) bool { return pr.CanAdminProject() }

// may reports whether pr may open report, and whether the answer is trustworthy.
func (g gate) may(ctx context.Context, pr principal.Principal, report string) (allowed, known bool) {
	if g.grants == nil || g.bypasses(pr) {
		return access.Allowed(pr, nil, nil), true
	}

	all, err := g.grants.Grants(ctx, pr.OrgID, pr.ProjectID)
	if err != nil {
		g.log.Error("could not read report grants", "err", err,
			"org", pr.OrgID, "project", pr.ProjectID)
		return false, false
	}
	mine := access.For(report, all)
	if len(mine) == 0 {
		// Nobody restricted this one, so the caller's groups do not matter and
		// the second query is not worth making. The common case by a distance:
		// most reports in most projects are never granted.
		return access.Allowed(pr, nil, nil), true
	}

	groups, err := g.grants.GroupsOf(ctx, pr.OrgID, pr.ProjectID, pr.Subject)
	if err != nil {
		g.log.Error("could not read group membership", "err", err, "subject", pr.Subject)
		return false, false
	}
	return access.Allowed(pr, mine, groups), true
}

/*
visible narrows a list of report names to the ones pr may open.

One pair of queries for the whole list rather than a pair per report: a
catalogue is the one place this is asked in bulk, and asking per row is how a
permission check becomes the reason a page is slow.
*/
func (g gate) visible(ctx context.Context, pr principal.Principal, reports []string) (map[string]bool, bool) {
	out := make(map[string]bool, len(reports))
	if g.grants == nil || g.bypasses(pr) {
		for _, name := range reports {
			out[name] = access.Allowed(pr, nil, nil)
		}
		return out, true
	}

	all, err := g.grants.Grants(ctx, pr.OrgID, pr.ProjectID)
	if err != nil {
		g.log.Error("could not read report grants", "err", err,
			"org", pr.OrgID, "project", pr.ProjectID)
		return nil, false
	}

	var groups []string
	if len(all) > 0 {
		if groups, err = g.grants.GroupsOf(ctx, pr.OrgID, pr.ProjectID, pr.Subject); err != nil {
			g.log.Error("could not read group membership", "err", err, "subject", pr.Subject)
			return nil, false
		}
	}
	for _, name := range reports {
		out[name] = access.Allowed(pr, access.For(name, all), groups)
	}
	return out, true
}
