/*
Package boot assembles a running cronos server from configuration.

Extracted from cmd/cronosd, which held all of it in package main — so the
enterprise binary, which is the same server with ee/ imported for its side
effects, could not run it and was a stub that printed two names and exited.

This package may never import ee/. The seams are in internal/extension and are
filled at init time by whichever binary imported what; nothing here knows which
build it is in, which is what makes the licence boundary a property of the
import graph rather than of anybody's discipline.
*/
package boot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	/*
	   The timezone database, carried in the binary.

	   A schedule's "first of the month at six" is a local claim, so cronos
	   calls time.LoadLocation on whatever a definition names — and without a
	   zoneinfo database that returns an error rather than, as the Dockerfile
	   used to claim, quietly resolving to UTC. Which means an instance with a
	   Europe/Berlin schedule refuses to start at all: loud, correct, and
	   discovered by whoever deployed it.

	   Our own image installs tzdata, but cronos is deployed by other people on
	   base images they choose, and the docs tell a federating deployment to
	   build its own. Embedding costs about 450KB in a 26MB binary and makes the
	   guarantee travel with the code instead of with the base image.

	   It is a fallback, not a replacement: Go reads the system database first
	   where there is one, so a host that updates tzdata for a DST law change
	   still wins. This is only what happens when there is nothing to read.
	*/
	_ "time/tzdata"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/adapter/driver/registry"
	"github.com/gsoultan/cronos/internal/extension"
	"github.com/gsoultan/cronos/internal/platform/build"
	"github.com/gsoultan/cronos/internal/platform/config"
	"github.com/gsoultan/cronos/internal/platform/token"

	_ "github.com/jackc/pgx/v5/stdlib"
	// Pure Go, so it costs a binary a megabyte and no cgo. DuckDB is behind a
	// build tag because it is not; this does not need to be.
	_ "github.com/microsoft/go-mssqldb"
	_ "modernc.org/sqlite"
)

// Serve runs until interrupted.
func Serve(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Every failure below is a configuration failure, and each one is raised
	// before the listener opens. A server that starts and then rejects every
	// request looks like a broken deployment rather than a missing variable.
	signer, err := token.NewSigner(cfg.SigningKey)
	if err != nil {
		return err
	}
	which, err := tenants(cfg)
	if err != nil {
		return err
	}
	several := len(which) > 1

	/*
	   One runtime per project: its definitions, its connections, its runner.
	   Never shared, because a report resolved from one project and run against
	   another's warehouse is one customer's numbers on another customer's
	   screen.

	   Definitions are read before the store, because a file-backed store
	   writes into the directory it was loaded from and refreshes that
	   repository after a publish.
	*/
	ctx := context.Background()
	runtimes, err := load(cfg, which, several)
	if err != nil {
		return err
	}

	// One store for every project: it has been multi-tenant since it existed,
	// and scopes each statement by the organisation and project of whoever is
	// asking rather than by which process is running. It doubles as the run
	// recorder when it is a database; a directory has nowhere to write history.
	defs, records, closeStore, err := definitionStore(ctx, cfg, only(runtimes), log)
	if err != nil {
		return err
	}
	defer closeStore()

	if several && records == nil {
		// A directory holds one project, and there is no sensible reading of
		// one as three. Said here rather than discovered as three projects
		// sharing a definition.
		return fmt.Errorf(
			"CRONOS_PROJECTS names %d projects, which needs CRONOS_STORE_DSN: "+
				"a definitions directory holds one", len(which))
	}

	/*
	   The name a first run gave this deployment, before anything uses one.

	   /setup adopts a tenancy in memory and records it; this is where the
	   memory is restored. Without it the process comes back believing it serves
	   what CRONOS_ORG says, finds an empty store for that tenant, adopts the
	   definitions directory a second time under the old name, and answers "you
	   do not have access to this project" to the administrator who set it up —
	   on a deployment that was working before it restarted.

	   Before the reconcile below, because that is the step that would adopt the
	   directory again. Only for a single-project deployment: a process told to
	   serve three cannot be renamed by anybody, and there is no first run to do
	   it.
	*/
	if records != nil && !several {
		org, project, err := records.Tenancy(ctx)
		if err != nil {
			return fmt.Errorf("reading what this deployment is called: %w", err)
		}
		if org != "" && (org != cfg.Org || project != cfg.Project) {
			log.Info("adopted", "project", org+"/"+project,
				"configured", cfg.Org+"/"+cfg.Project)
			cfg.Org, cfg.Project = org, project
			runtimes = renamed(runtimes, tenant{org: org, project: project})
		}
	}

	for _, rt := range runtimes {
		if err := finish(ctx, cfg, rt, defs, records, log); err != nil {
			return err
		}
		defer rt.close() //nolint:errcheck // closing pools on the way out
	}

	// One recorder, however it is served: the counting happens in the handler
	// chain either way, and only where it can be read changes. Built before the
	// schedulers so each can register itself as it arms.
	// Wired here and only here, so the count can never be a zero nobody set —
	// which on this metric is the same shape as a healthy deployment.
	metrics := api.NewMetrics().CountingUnarmed(Unarmed).CountingRefused(Rejected)

	/*
	   Every project's connection pools, for every replica.

	   Registered here rather than beside the scheduler, because a replica that
	   schedules nothing still holds connections to somebody's warehouse — and
	   an operator asking "is the pool the ceiling" is asking about the instance
	   serving reports, which is all of them.
	*/
	for t, rt := range runtimes {
		// The concrete type, the same assertion readinessFor makes. Probes is
		// an interface a test can fill with something narrower, and a stub that
		// answers probes has no pool to report on.
		if reg, ok := rt.project.Probes.(*registry.Registry); ok && reg != nil {
			metrics.WatchPools(t.String(), reg)
		}
	}

	/*
	   Deferred beside the start, and it both cancels and waits.

	   The order matters and this is the order: listen below drains HTTP first,
	   then this runs. A scheduled run may reach this deployment's own API, so
	   stopping the schedulers first would fail the run the drain exists to let
	   finish.
	*/
	/*
	   Before the schedulers, because they ask it who they are.

	   A schedule runs as a project member, and which project that is can change
	   once — a deployment named through /setup is a different tenancy
	   afterwards. Built here so the scheduler reads the answer rather than
	   remembering the one it was born with.
	*/
	projects := projectsFor(runtimes, several)

	stopSchedulers, err := startSchedulers(cfg, runtimes, records, metrics, projects, log)
	defer stopSchedulers()
	if err != nil {
		return err
	}

	// Before the first request can arrive, and said out loud in the line
	// below: a deployment recording nothing should be a deployment somebody
	// chose, not one they ended up with.
	auditSink := auditing(cfg, log)

	// Run history is the only thing here that grows without bound, and the
	// page that reads it is the one somebody opens when something is wrong.
	retention, stopRetention := context.WithCancel(context.Background())
	defer stopRetention()
	retain(retention, records, cfg, log)
	// Whatever the history retention, an expired invitation is dead weight
	// holding an address and the hash of a credential.
	sweepInvitations(retention, records, log)

	handler := api.Routes(api.Deps{
		Projects: projects, Signer: signer,
		Origins: cfg.Origins, Log: log,

		Publish: publishingBy(projects, runtimes),
		Store:   defs,
		Admin:   api.NewAdminKey(cfg.AdminKey, cfg.Org, cfg.Project),

		Runs:      history(records),
		Users:     users(records),
		Shares:    sharingFor(records, signer, projects, runtimes),
		Roster:    roster(records),
		Directory: directory(records),

		Factors:     factors(records),
		Platform:    platform(records),
		Policies:    policies(records),
		Accounts:    accounts(records),
		Invitations: invitations(records),
		Post:        postman(cfg, log),
		Portal:      cfg.Portal,
		Sends:       sendingFor(cfg, projects, runtimes, log),
		Channels:    channelNames(cfg, log),

		Ready:   readinessFor(records, runtimes),
		Metrics: metrics, MetricsElsewhere: cfg.MetricsAddr != "",

		Org: cfg.Org, Project: cfg.Project,
		BehindProxy: cfg.BehindProxy,
	})

	// The build first, because it is the field somebody greps for when a fleet
	// is halfway through a rollout and one instance is behaving differently
	// from the rest.
	log.Info("cronosd listening",
		"build", build.Version(),
		"addr", cfg.Addr, "driver", cfg.Driver,
		"projects", names(which), "definitions", counts(runtimes),
		"origins", cfg.Origins, "management", len(cfg.AdminKey) > 0,
		"scheduler", cfg.Scheduler, "sign-in", records != nil,
		"auth", extension.Auth().Name(), "audit", auditSink)

	/*
	   Metrics on an address of their own, when asked for.

	   Started before the API rather than after, so a deployment that cannot
	   bind the metrics address finds out at startup instead of discovering an
	   unscraped instance later. Drained on the way out by the same defer that
	   closes everything else.
	*/
	if cfg.MetricsAddr != "" {
		stopMetrics, err := serveMetrics(cfg.MetricsAddr, metrics, log)
		if err != nil {
			return err
		}
		defer stopMetrics()
	}

	return listen(cfg.Addr, handler, log)
}

/*
serveMetrics puts the exposition on its own listener.

A second server rather than a path on the first, because what it answers is not
for the same audience: the API is reached by browsers and by an ISV's
application, and the exposition is scraped by something on the operator's own
network. Binding it to 127.0.0.1 or to a private interface is then a deployment
decision rather than an ingress rule somebody has to remember.

No timeouts to speak of and no drain worth naming: this serves one route to one
scraper, and a request in flight at shutdown is a scrape that will happen again
in fifteen seconds.
*/
func serveMetrics(addr string, h http.Handler, log *slog.Logger) (func(), error) {
	mux := http.NewServeMux()
	mux.Handle("/v1/metrics", h)
	// The same path as on the main listener, so moving it is one environment
	// variable rather than a change to every scrape config.
	mux.Handle("/metrics", h)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("metrics: %w", err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics listener stopped", "err", err)
		}
	}()
	log.Info("metrics", "addr", addr, "onTheApi", false)

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}, nil
}

// seed applies a .sql file, for a development database that starts empty.
func seed(db *sql.DB, path string, log *slog.Logger) error {
	if path == "" {
		return nil
	}
	script, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := db.Exec(string(script)); err != nil {
		return err
	}
	log.Info("seeded", "file", path)
	return nil
}

// listen serves until interrupted, then drains.
//
// In-flight reports finish. Killing a burst mid-render leaves a delivery that
// half happened, which is worse to reconcile than one that did not start.
func listen(addr string, h http.Handler, log *slog.Logger) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: h,

		// Headers, so a connection that opens and says nothing cannot hold a
		// goroutine for ever. The classic slow-loris.
		ReadHeaderTimeout: 10 * time.Second,

		/*
		   The body too, which the header timeout does not cover.

		   Every handler bounds how large a body may be; none of them bounds how
		   long it may take to arrive. A client sending four kilobytes at a byte
		   a second holds a goroutine and a connection for over an hour, and a
		   thousand of them cost nothing to open.

		   A minute is generous for the largest thing this API accepts, which is
		   a definition somebody typed.
		*/
		ReadTimeout: time.Minute,

		/*
		   And a keep-alive connection nobody is using is closed.

		   This is the one whose absence is invisible: with ReadTimeout unset,
		   IdleTimeout falls back to it, and both were zero — so a connection
		   that finished its request and went quiet was held until the operating
		   system noticed, which for a client that simply vanished is never.
		   Behind a load balancer that opens a pool per instance, the count only
		   goes up.

		   Two minutes, above the 60-second idle most proxies use, so the proxy
		   is the one that closes and no request lands on a connection this side
		   is retiring.
		*/
		IdleTimeout: 2 * time.Minute,

		/*
		   WriteTimeout is deliberately not set.

		   It bounds the whole exchange, not just the write, so any value would
		   be a cap on how long a report may take — and a report is a query
		   against somebody's warehouse followed by a typesetter. A monthly
		   statement over a large customer legitimately takes longer than
		   anything that could be defended as a default here.

		   What bounds the work is bounded where the work is: the executor
		   carries the datasource's own statement timeout, and the client's
		   disconnect cancels the query.
		*/
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()

	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stop:
		log.Info("draining")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
