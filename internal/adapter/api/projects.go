package api

import (
	"context"
	"fmt"
	"sync"

	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Project is one tenant's runtime: its definitions, its connections, and the
things that read them.

Held together because they belong together and must never be mixed. A report
resolved from one project and run against another's warehouse is the worst
failure this product has — it is not an error anybody sees, it is one
customer's numbers on another customer's screen — so the five of them are
resolved once, from the principal, and handed around as a unit rather than
threaded separately and paired by hand at each call site.
*/
type Project struct {
	Reports     Reports
	Runner      *run.Service
	Definitions Repository
	Due         Due
	Fires       Firing
	Probes      Probes
}

/*
Projects resolves the runtime a request acts in.

A port rather than a field, because "which projects does this process serve" is
a deployment's answer. One is the ordinary case and stays exactly as cheap as
it was — see One, which answers the same runtime whatever it is asked.
*/
type Projects interface {
	Project(ctx context.Context, pr principal.Principal) (*Project, error)
}

/*
One is a Projects that serves a single project.

What every deployment was before this existed, and what most stay. Written here
rather than in each caller so "this process serves one project" is a stated
configuration rather than an assumption spread across the code — and so the
multi-tenant path is exercised by the same handlers rather than being a
different set of them.
*/
type One struct {
	Org, ProjectID string
	Only           *Project

	/*
	   adopted is the tenancy a first run named, when there was none.

	   A fresh install serves whatever CRONOS_ORG and CRONOS_PROJECT default to
	   — "default/default" — because nothing else exists yet. Then somebody
	   opens /setup and calls the organisation Acme Logistics, and their account
	   is created there. Without this, that account signs in successfully and
	   sees nothing at all: its principal says acme-logistics/finance and the
	   process says default/default, so every read is refused by the very check
	   that keeps tenants apart.

	   Discovering it from the database would be the other answer, and Many's
	   comment explains why not: a project appearing in a database is not a
	   reason for a process to open connections to warehouses nobody named. This
	   is narrower — it is the deployment being told, once, at the moment
	   somebody sets it up, by the endpoint that can only run once.
	*/
	mu       sync.RWMutex
	adopted  bool
	adoptOrg string
	adoptPrj string
}

/*
Adopt names the tenancy of a deployment that had none.

Called by the first run and by nothing else. It is refused once anything has
been adopted, and it is refused when the deployment was configured explicitly —
a process told to serve acme/finance must not be renamed by an HTTP request, and
the only reason this exists at all is the case where nothing was told to it.
*/
func (o *One) Adopt(org, project string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.adopted || org == "" || project == "" {
		return false
	}
	o.adopted, o.adoptOrg, o.adoptPrj = true, org, project
	return true
}

// serving is the tenancy this process answers for.
func (o *One) serving() (string, string) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.adopted {
		return o.adoptOrg, o.adoptPrj
	}
	return o.Org, o.ProjectID
}

// Project returns the one runtime, having checked the caller belongs to it.
//
// Checked, and not merely returned: a single-project process that served its
// definitions to a principal from another organisation would be the same leak
// as a multi-tenant one that resolved the wrong runtime. The narrowness of the
// deployment is not the check.
func (o *One) Project(_ context.Context, pr principal.Principal) (*Project, error) {
	org, project := o.serving()
	if pr.OrgID != org || pr.ProjectID != project {
		return nil, fmt.Errorf("%w: this server holds %s/%s", ErrNoProject, org, project)
	}
	return o.Only, nil
}

/*
Many is a Projects over several runtimes, keyed by tenant.

Built once at startup from the projects a deployment named. Nothing is
discovered: a project appearing in a database is not a reason for a process to
start opening connections to warehouses nobody told it about, and adding one is
a deploy rather than an INSERT.
*/
type Many struct {
	projects map[string]*Project
}

// NewMany indexes runtimes by organisation and project.
func NewMany() *Many { return &Many{projects: map[string]*Project{}} }

// Add registers one.
func (m *Many) Add(org, project string, p *Project) {
	m.projects[key(org, project)] = p
}

// Project resolves the runtime for this principal, and only for this one.
//
// From the principal's own organisation and project, which came from a signed
// token or a checked admin key. There is deliberately no way to ask for a
// different project: an argument that selected one would be a parameter an
// attacker supplies, and every other tenancy decision in this codebase is made
// the same way for the same reason. See docs/tenancy.md.
func (m *Many) Project(_ context.Context, pr principal.Principal) (*Project, error) {
	if pr.OrgID == "" || pr.ProjectID == "" {
		return nil, fmt.Errorf("%w: the caller names no project", ErrNoProject)
	}
	p, ok := m.projects[key(pr.OrgID, pr.ProjectID)]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrNoProject, pr.OrgID, pr.ProjectID)
	}
	return p, nil
}

// Names lists what this process serves, for a startup line and a readiness
// check.
func (m *Many) Names() []string {
	out := make([]string, 0, len(m.projects))
	for name := range m.projects {
		out = append(out, name)
	}
	return out
}

// key joins the two halves of a tenant with a character neither may contain.
//
// A separator that could appear in either would let `acme/finance` and
// `acme/fin` + `ance` collide, which is a cross-tenant read produced by string
// concatenation.
func key(org, project string) string { return org + "\x00" + project }
