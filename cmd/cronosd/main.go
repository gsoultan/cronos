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
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/extension"
	"github.com/gsoultan/cronos/internal/platform/config"
	"github.com/gsoultan/cronos/internal/platform/token"

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
	engines, closeEngines, err := datasources(cfg, repo, log)
	if err != nil {
		return err
	}
	defer closeEngines()

	runner := run.New(repo, engines)

	// One store, opened before the scheduler so a burst's first run is
	// recorded. It doubles as the run recorder when it is a database; when it
	// is a directory there is nowhere to write history and records is nil.
	defs, records, closeStore, err := definitionStore(context.Background(), cfg, repo, log)
	if err != nil {
		return err
	}
	defer closeStore()

	if cfg.Scheduler {
		sched, err := scheduler(cfg, repo, runner, records, log)
		if err != nil {
			return err
		}
		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		go func() {
			if err := sched.Start(ctx); err != nil {
				log.Error("scheduler stopped", "err", err)
			}
		}()
	}

	handler := api.RoutesWith(repo, runner, signer, cfg.Origins, log,
		publish.New(defs, repo).WithReports(repo), defs,
		api.NewAdminKey(cfg.AdminKey, cfg.Org, cfg.Project), history(records), users(records))

	datasets, reports, schedules, sources := repo.Counts()
	log.Info("cronosd listening",
		"addr", cfg.Addr, "driver", cfg.Driver,
		"datasets", datasets, "reports", reports, "schedules", schedules, "sources", sources,
		"origins", cfg.Origins, "management", len(cfg.AdminKey) > 0,
		"scheduler", cfg.Scheduler, "sign-in", records != nil,
		"auth", extension.Auth().Name(), "audit", extension.Audit().Name())

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
