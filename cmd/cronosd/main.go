// Command cronosd is the Business Source License build of the cronos server.
//
// It must not import github.com/gsoultan/cronos/ee, directly or transitively.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/adapter/store/file"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/extension"
	"github.com/gsoultan/cronos/internal/platform/config"
	"github.com/gsoultan/cronos/internal/platform/token"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := serve(log); err != nil {
		log.Error("cronosd stopped", "err", err)
		os.Exit(1)
	}
}

func serve(log *slog.Logger) error {
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
	repo, err := file.Load(cfg.Definitions)
	if err != nil {
		return err
	}

	// The store, opened before anything reads a definition. It doubles as the
	// run recorder when it is a database; when it is a directory there is
	// nowhere to write history and records is nil.
	ctx := context.Background()
	defs, records, closeStore, err := definitionStore(ctx, cfg, repo, log)
	if err != nil {
		return err
	}
	defer closeStore()

	// Before the connections are opened, because the store decides which
	// sources exist: building engines from the directory and then adopting a
	// different set would leave every dataset pointing at a pool nobody made.
	if records != nil {
		if err := reconcile(ctx, defs, repo, cfg.Org, cfg.Project, log); err != nil {
			return err
		}
	}

	engines, closeEngines, err := datasources(cfg, repo, log)
	if err != nil {
		return err
	}
	defer closeEngines()

	runner := run.New(repo, engines)

	// armed answers when each schedule next fires; firing runs one now. Both
	// are the same service, named apart so a handler is given only the verb it
	// needs.
	var armed api.Due
	var firing api.Firing
	if cfg.Scheduler {
		sched, err := scheduler(cfg, repo, runner, records, log)
		if err != nil {
			return err
		}
		// So the catalogue can say when each schedule next fires, and say
		// nothing rather than a time nothing will honour when it cannot.
		armed, firing = sched, sched
		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		go func() {
			if err := sched.Start(ctx); err != nil {
				log.Error("scheduler stopped", "err", err)
			}
		}()
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

	handler := api.Routes(api.Deps{
		Reports: repo, Runner: runner, Signer: signer,
		Origins: cfg.Origins, Log: log,

		Publish: publishing(defs, repo, records),
		Store:   defs,
		Admin:   api.NewAdminKey(cfg.AdminKey, cfg.Org, cfg.Project),

		Definitions: repo,
		Due:         armed,
		Runs:        history(records),
		Users:       users(records),
		Fires:       firing,
		Shares:      sharing(records, signer, repo),
		Probes:      probing(engines),

		Ready:   readiness(records, engines),
		Metrics: api.NewMetrics(),

		Org: cfg.Org, Project: cfg.Project,
		BehindProxy: cfg.BehindProxy,
	})

	datasets, reports, schedules, sources := repo.Counts()
	log.Info("cronosd listening",
		"addr", cfg.Addr, "driver", cfg.Driver,
		"datasets", datasets, "reports", reports, "schedules", schedules, "sources", sources,
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
