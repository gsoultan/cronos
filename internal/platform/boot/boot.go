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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/extension"
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

	for _, rt := range runtimes {
		if err := finish(ctx, cfg, rt, defs, records, log); err != nil {
			return err
		}
		defer rt.close() //nolint:errcheck // closing pools on the way out
	}

	schedules, stopSchedulers := context.WithCancel(context.Background())
	defer stopSchedulers()
	if err := startSchedulers(schedules, cfg, runtimes, records, log); err != nil {
		return err
	}

	projects := projectsFor(runtimes, several)

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

		Publish: publishingFor(runtimes),
		Store:   defs,
		Admin:   api.NewAdminKey(cfg.AdminKey, cfg.Org, cfg.Project),

		Runs:      history(records),
		Users:     users(records),
		Shares:    sharingFor(records, signer, runtimes),
		Roster:    roster(records),
		Directory: directory(records),

		Factors:     factors(records),
		Platform:    platform(records),
		Accounts:    accounts(records),
		Invitations: invitations(records),
		Post:        postman(cfg, log),
		Portal:      cfg.Portal,
		Sends:       sendingFor(cfg, runtimes, log),
		Channels:    channelNames(cfg, log),

		Ready:   readinessFor(records, runtimes),
		Metrics: api.NewMetrics(),

		Org: cfg.Org, Project: cfg.Project,
		BehindProxy: cfg.BehindProxy,
	})

	log.Info("cronosd listening",
		"addr", cfg.Addr, "driver", cfg.Driver,
		"projects", names(which), "definitions", counts(runtimes),
		"origins", cfg.Origins, "management", len(cfg.AdminKey) > 0,
		"scheduler", cfg.Scheduler, "sign-in", records != nil,
		"auth", extension.Auth().Name(), "audit", auditSink)

	return listen(cfg.Addr, handler, log)
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
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
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
