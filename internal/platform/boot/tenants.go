package boot

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/adapter/driver/registry"
	"github.com/gsoultan/cronos/internal/adapter/store/file"
	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/app/schedule"
	"github.com/gsoultan/cronos/internal/app/send"
	"github.com/gsoultan/cronos/internal/app/share"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/config"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
Assembling one runtime per project.

The store has been multi-tenant since it existed — every statement is scoped by
organisation and project taken from the caller's identity. The runtime was not:
the definitions in memory, the connection pools, and the scheduler were all one
set, pinned by CRONOS_ORG and CRONOS_PROJECT, so a deployment serving three
projects ran three processes.

What that bought was a guarantee worth naming before giving it up: the blast
radius of a bad definition, a runaway query or a leaked signing key was one
customer's project, because there was physically nothing else in the process.
Serving several means that isolation is now a property of the code — every
handler resolves its runtime from the caller's own principal — rather than of
the operating system. One process per project remains supported and is still
the right answer for a deployment that can afford it.
*/

// tenant names one project a process serves.
type tenant struct{ org, project string }

func (t tenant) String() string { return t.org + "/" + t.project }

/*
tenants is what this process was told to serve.

From configuration and never from discovery. A project appearing in a database
is not a reason for a process to start opening connections to warehouses nobody
told it about, and a deployment that grows a tenant should be a deploy rather
than an INSERT somebody made at four in the afternoon.
*/
func tenants(cfg config.Server) ([]tenant, error) {
	if strings.TrimSpace(cfg.Projects) == "" {
		// The ordinary deployment, unchanged: one project, named the way it
		// always was.
		return []tenant{{org: cfg.Org, project: cfg.Project}}, nil
	}

	var out []tenant
	for _, pair := range strings.Split(cfg.Projects, ",") {
		org, project, ok := strings.Cut(strings.TrimSpace(pair), "/")
		if !ok || org == "" || project == "" {
			return nil, fmt.Errorf(
				"CRONOS_PROJECTS: %q is not org/project", strings.TrimSpace(pair))
		}
		out = append(out, tenant{org: org, project: project})
	}
	return out, nil
}

/*
definitionsFor is where one project's files live.

Under the configured directory, in org/project, so a deployment serving several
has one tree with a subdirectory each rather than an environment variable per
tenant. A single-project deployment keeps its directory exactly where it was —
naming one project must not move anybody's files.
*/
func definitionsFor(cfg config.Server, t tenant, several bool) string {
	if !several {
		return cfg.Definitions
	}
	return filepath.Join(cfg.Definitions, t.org, t.project)
}

// runtime is everything one project needs, and how to close it.
type runtime struct {
	tenant  tenant
	project *api.Project
	repo    *file.Repository
	publish *publish.Service
	close   func() error
}

/*
load reads each project's definitions.

Before the store, because a file-backed store writes into the directory it was
loaded from and needs the repository to refresh after a publish. A database
store needs none of this and is built from nothing — which is why serving
several projects requires one: a directory holds one project, and there is no
sensible reading of one directory as three.
*/
func load(cfg config.Server, which []tenant, several bool) (map[tenant]*runtime, error) {
	out := map[tenant]*runtime{}
	for _, t := range which {
		where := definitionsFor(cfg, t, several)
		repo, err := file.Load(where)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		out[t] = &runtime{tenant: t, repo: repo}
	}
	return out, nil
}

/*
finish reconciles a project against the store and opens its connections.

The same sequence the single-tenant boot ran, per project. Nothing is shared
between two of these except the store, which scopes every statement itself.
*/
func finish(ctx context.Context, cfg config.Server, rt *runtime,
	defs publish.Store, records *sqlstore.Store, log *slog.Logger) error {

	// Before the connections are opened, because the store decides which
	// sources exist: building engines from the directory and then adopting a
	// different set would leave every dataset pointing at a pool nobody made.
	if records != nil {
		if err := reconcile(ctx, defs, rt.repo, rt.tenant.org, rt.tenant.project, log); err != nil {
			return fmt.Errorf("%s: %w", rt.tenant, err)
		}
	}

	engines, closeEngines, err := datasources(cfg, rt.repo, log)
	if err != nil {
		return fmt.Errorf("%s: %w", rt.tenant, err)
	}

	rt.project = &api.Project{
		Reports:     rt.repo,
		Runner:      run.New(rt.repo, engines),
		Definitions: rt.repo,
		Probes:      probing(engines),
	}
	rt.publish = publishing(defs, rt.repo, records, engines)
	rt.close = closeEngines
	return nil
}

/*
startSchedulers arms a scheduler per project and returns a wait for them.

One each rather than one over all of them: a schedule runs as its project's
owner, against its project's datasources, and a single loop would need to
resolve both per firing — which is the same resolution done once here, where it
can be read.

The wait is the point, and it did not exist. Every scheduler's Start already
holds a WaitGroup over its in-flight runs and blocks on it when the context is
cancelled — so a burst mid-delivery finishes rather than being abandoned. That
guarantee was unreachable: nothing kept a handle on these goroutines, so the
process cancelled them and returned, and the runtime tore down the goroutine
that was waiting.

The effect was the one the drain exists to prevent, on the path that matters
most. cronos is a report scheduler; the work it exists to do happens in these
goroutines, not in an HTTP handler. A rolling deploy at six in the morning on
the first of the month lands exactly on the monthly statements burst, and half
a customer list receives a document while the other half does not — the state
that is worst to reconcile, because nobody can tell from outside which half.
*/
func startSchedulers(cfg config.Server, runtimes map[tenant]*runtime,
	records *sqlstore.Store, watch *api.Metrics, projects api.Projects,
	log *slog.Logger) (stop func(), err error) {

	/*
	   One thing to call, and it both cancels and waits.

	   Two — a cancel and a separate wait — is what this was, and the wait was
	   the one that went missing: boot cancelled, returned, and the runtime tore
	   down the goroutine doing the waiting. Returning a single stop makes that
	   particular mistake unwritable, and it is deferred beside the start rather
	   than called sixty lines below, where a `return` added later would skip it.
	*/
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var leases []*sqlstore.Lease
	stop = func() {
		cancel()
		drain(wg.Wait, log)
		// After the drain, not before: a burst finishing is still this
		// instance's work, and handing leadership over while it delivers would
		// let a replacement start the same schedule beside it.
		for _, l := range leases {
			l.Release()
		}
	}

	/*
	   Where the tenancy can change, the scheduler asks rather than remembers.

	   Only api.One can be adopted — a multi-project deployment names each of
	   them in configuration and none of those can be renamed by a request. So
	   this is nil for Many, where asking would be a slower way to get the same
	   answer.
	*/
	var serving func() (string, string)
	if one, ok := projects.(*api.One); ok {
		serving = one.Serving
	}

	for t, rt := range runtimes {
		sched, err := scheduler(cfg, t.org, t.project, serving, rt.repo, rt.project.Runner, records, log)
		if err != nil {
			// The schedulers already started keep running until the caller
			// stops them; returning stop alongside the error means it can.
			return stop, fmt.Errorf("%s: %w", t, err)
		}

		/*
		   Resuming works whether or not this process schedules.

		   Built before the CRONOS_SCHEDULER check on purpose. Repairing a
		   partly-delivered burst is an operator action, and making it work only
		   on the replica that happens to be leading would mean hunting for that
		   replica during the incident. A deployment that has turned scheduling
		   off entirely — often because something went wrong — still has the
		   partial runs to repair.

		   Nothing fires by itself here: the loop below is what does that, and
		   it only starts when asked.
		*/
		if records != nil {
			rt.project.Resumes = resumer{records: records, sched: sched}
		}

		if !cfg.Scheduler {
			continue
		}
		/*
		   Leadership, where the store can arbitrate it.

		   Every replica may now run with CRONOS_SCHEDULER=1 and exactly one
		   fires. Before this, that was a rule a deployment held in its head:
		   set it twice and every customer gets two statements, forget it and
		   nobody gets one — and both are quiet, because the only party who
		   notices is the recipient.

		   Nil for a file-backed or SQLite deployment, which is one process by
		   construction and leads unconditionally.
		*/
		if records != nil {
			if lease := records.Lease("scheduler:" + t.String()); lease != nil {
				sched = sched.WithElection(lease)
				leases = append(leases, lease)
			}
		}

		rt.project.Due, rt.project.Fires = sched, sched
		if watch != nil {
			// Registered only for a scheduler that was actually armed, so no
			// watcher at all is itself the signal that this process schedules
			// nothing — ordinary for a replica, an incident when it is true of
			// every replica at once.
			watch.WatchScheduler(t.String(), sched)
		}

		wg.Add(1)
		go func(t tenant, sched interface{ Start(context.Context) error }) {
			defer wg.Done()
			if err := sched.Start(ctx); err != nil {
				log.Error("scheduler stopped", "project", t.String(), "err", err)
			}
		}(t, sched)
	}
	return stop, nil
}

/*
drain waits for in-flight scheduled runs, up to a bound.

The bound is longer than the HTTP drain because the work is longer: a burst is
one render per recipient and one delivery each, where a request is one render.
It is shorter than the grace period an orchestrator gives by default —
Kubernetes allows thirty seconds before SIGKILL, and a drain that outlives that
never completes. It is only a way to be killed mid-burst with an extra half
minute of confusion first.
*/
func drain(wait func(), log *slog.Logger) {
	if wait == nil {
		return
	}
	done := make(chan struct{})
	go func() { wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(schedulerDrain):
		// Said out loud rather than swallowed. A burst that did not finish is
		// a delivery somebody has to reconcile, and the log is where they will
		// look for the reason.
		log.Warn("scheduled runs did not finish before shutdown", "waited", schedulerDrain)
	}
}

// schedulerDrain is how long in-flight scheduled runs get after SIGTERM.
const schedulerDrain = 25 * time.Second

// projectsFor turns the assembled runtimes into what the API resolves against.
func projectsFor(runtimes map[tenant]*runtime, several bool) api.Projects {
	if !several {
		for t, rt := range runtimes {
			// api.One still checks the caller belongs to it. A single-project
			// process that served its definitions to a principal from another
			// organisation would be the same leak as a multi-tenant one
			// resolving the wrong runtime; the narrowness is not the check.
			return &api.One{Org: t.org, ProjectID: t.project, Only: rt.project}
		}
	}
	many := api.NewMany()
	for t, rt := range runtimes {
		many.Add(t.org, t.project, rt.project)
	}
	return many
}

// only returns the single project's repository, for a file-backed store.
//
// Nil where there are several, which is the case that store cannot serve — and
// the caller says so rather than passing one of three and hoping.
func only(runtimes map[tenant]*runtime) *file.Repository {
	if len(runtimes) != 1 {
		return nil
	}
	for _, rt := range runtimes {
		return rt.repo
	}
	return nil
}

// names lists what this process serves, for the startup line.
func names(which []tenant) []string {
	out := make([]string, 0, len(which))
	for _, t := range which {
		out = append(out, t.String())
	}
	return out
}

// counts is how much each project holds, so the startup line says whether the
// definitions somebody expected actually loaded.
func counts(runtimes map[tenant]*runtime) map[string]string {
	out := map[string]string{}
	for t, rt := range runtimes {
		datasets, reports, schedules, sources := rt.repo.Counts()
		out[t.String()] = fmt.Sprintf("%d datasets, %d reports, %d schedules, %d sources",
			datasets, reports, schedules, sources)
	}
	return out
}

/*
The services that were one and are now one per project.

Each takes the caller's runtime rather than the process's, and each is a thin
resolver over the map — the work of building them stayed where it was, in
wiring.go, and this is only the part that says whose.
*/

// publishingFor validates and stores against the caller's own project.
//
// Each has its own datasets to check a report against and its own engines to
// prove a block will run, so publishing into the wrong one would accept a
// report naming a dataset this project does not have.
/*
publishingFor resolves through the same object every other route does.

It used to key its own map by the caller's organisation and project, and that is
the bug a first run exposed. A fresh install serves whatever CRONOS_ORG defaults
to; somebody sets it up as "acme/finance"; api.One adopts the name so reads work
— and this map, built at boot, still had one entry under default/default. Every
publish, send and share answered "no such project here". A deployment set up
through the browser could read its reports and change nothing.

Three helpers had their own copy of "which tenant is this", so the fix is not to
teach three maps about adoption. It is to have one thing decide, which is what
api.Projects already is, and to key these by the runtime it hands back.
*/
type publishingFor struct {
	projects api.Projects
	// byProject maps the resolved runtime to its publisher. Keyed by pointer
	// identity rather than by name, so nothing here has an opinion about
	// tenancy at all.
	byProject map[*api.Project]*publish.Service
}

func (p publishingFor) of(ctx context.Context, pr principal.Principal) (*publish.Service, error) {
	project, err := p.projects.Project(ctx, pr)
	if err != nil {
		return nil, fmt.Errorf("%w: no such project here", publish.ErrForbidden)
	}
	svc, ok := p.byProject[project]
	if !ok {
		return nil, fmt.Errorf("%w: nothing publishes here", publish.ErrForbidden)
	}
	return svc, nil
}

func (p publishingFor) Publish(ctx context.Context, raw []byte,
	pr principal.Principal) (publish.Result, error) {

	svc, err := p.of(ctx, pr)
	if err != nil {
		return publish.Result{}, err
	}
	return svc.Publish(ctx, raw, pr)
}

func (p publishingFor) Delete(ctx context.Context, pr principal.Principal, kind, name string) error {
	svc, err := p.of(ctx, pr)
	if err != nil {
		return err
	}
	return svc.Delete(ctx, pr, kind, name)
}

// sharingFor mints links against the caller's own reports.
//
// The report a link names must exist in the project the sharer acts in, which
// is what stops a link to a name that happens to exist somewhere else.
// sharingFor resolves the same way, so a link minted after a first run names a
// report in the project the caller is actually in.
func sharingFor(records *sqlstore.Store, signer *token.Signer,
	projects api.Projects, runtimes map[tenant]*runtime) api.Sharing {

	if records == nil {
		return nil
	}
	reports := map[*api.Project]*file.Repository{}
	for _, rt := range runtimes {
		reports[rt.project] = rt.repo
	}
	return share.NewPerProject(records, signer, func(pr principal.Principal) share.Reports {
		project, err := projects.Project(context.Background(), pr)
		if err != nil {
			return nil
		}
		if repo, ok := reports[project]; ok {
			return repo
		}
		return nil
	})
}

// sendingFor renders and delivers from the caller's own project.
func sendingFor(cfg config.Server, projects api.Projects,
	runtimes map[tenant]*runtime, log *slog.Logger) api.Sending {

	chans, err := channels(cfg, log)
	if err != nil || len(chans) == 0 {
		return nil
	}
	services := map[*api.Project]*send.Service{}
	for _, rt := range runtimes {
		services[rt.project] = send.New(rt.repo, documents(rt.project.Runner), chans...)
	}
	return sendPerProject{projects: projects, byProject: services}
}

// sendPerProject resolves the same way publishingFor does, and for the same
// reason: its own map of tenants was a second place for "which project is this"
// to be answered, and a first run made the two disagree.
type sendPerProject struct {
	projects  api.Projects
	byProject map[*api.Project]*send.Service
}

func (s sendPerProject) Send(ctx context.Context, req send.Request,
	pr principal.Principal) (send.Result, error) {

	/*
	   Forbidden rather than invalid, which publishingFor next door has always
	   said and this said until now.

	   The request is well formed; the caller is somewhere else. Saying "not a
	   send" makes it a 400, which tells somebody their request was malformed
	   when the truth is that they are in the wrong project — and it made the
	   two sentinels indistinguishable to a caller trying to tell a bad request
	   from a tenancy refusal.
	*/
	project, err := s.projects.Project(ctx, pr)
	if err != nil {
		return send.Result{}, fmt.Errorf("%w: no such project here", send.ErrForbidden)
	}
	svc, ok := s.byProject[project]
	if !ok {
		return send.Result{}, fmt.Errorf("%w: nothing sends here", send.ErrForbidden)
	}
	return svc.Send(ctx, req, pr)
}

// readinessFor asks the store once and every project's datasources by name.
//
// Named by project, because "a datasource is unreachable" is not an answer
// somebody can act on when a process serves three of them and two are fine.
func readinessFor(records *sqlstore.Store, runtimes map[tenant]*runtime) []api.Check {
	var checks []api.Check
	if records != nil {
		checks = append(checks, storeCheck(records))
	}
	for t, rt := range runtimes {
		probes, ok := rt.project.Probes.(*registry.Registry)
		if !ok {
			continue
		}
		for _, name := range probes.Names() {
			checks = append(checks, api.Check{
				Name: "datasource:" + t.String() + ":" + name,
				Probe: func(ctx context.Context) error {
					_, err := probes.Probe(ctx, name)
					return err
				},
			})
		}
	}
	return checks
}

// publishing builds the resolver, once the projects are known.
func publishingBy(projects api.Projects, runtimes map[tenant]*runtime) api.Publishing {
	byProject := map[*api.Project]*publish.Service{}
	for _, rt := range runtimes {
		byProject[rt.project] = rt.publish
	}
	return publishingFor{projects: projects, byProject: byProject}
}

/*
resumer re-sends a period to whoever did not get it.

Here rather than in the API package because it is the only place that holds all
three pieces: the store that knows what was delivered, the scheduler that knows
the schedule, and the burst runner that does the sending. The API knows none of
them and asks through a port.

Absent without a store, which is a file-backed deployment: there is no run
history to resume from, and the endpoint answers 404 like any other run.
*/
type resumer struct {
	records *sqlstore.Store
	sched   *schedule.Service
}

func (r resumer) Resume(ctx context.Context, pr principal.Principal, runID string) error {
	// Tenant-scoped, so a run id from another project reads as absent rather
	// than as forbidden — the difference tells a caller it exists.
	run, _, err := r.records.Run(ctx, pr, runID)
	if err != nil {
		return err
	}

	/*
	   Every attempt at this period, not just the run being resumed.

	   A burst that was cut, resumed, and cut again has two sets of deliveries.
	   A third attempt reading only the run it was pointed at would send a
	   duplicate to everybody the other attempt reached — which is the failure
	   this whole path exists to prevent, arrived at by resuming twice.
	*/
	done, err := r.records.DeliveredFor(ctx, pr, run.Schedule, run.PeriodStart, run.PeriodEnd)
	if err != nil {
		return err
	}
	return r.sched.Resume(ctx, run.Schedule, run.PeriodStart, run.PeriodEnd, done, pr)
}
