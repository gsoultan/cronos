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
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/adapter/store/file"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/query"
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
	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := seed(db, cfg.Seed, log); err != nil {
		return err
	}

	dialect, err := dialectFor(cfg.Driver)
	if err != nil {
		return err
	}

	exec := sqldriver.NewExecutor(db)
	runner := run.New(repo, exec, query.NewBuilder(dialect))

	if cfg.Scheduler {
		sched, err := scheduler(cfg, repo, runner, exec, log)
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

	writer := file.NewWriter(cfg.Definitions, repo)
	handler := api.RoutesWith(repo, runner, signer, cfg.Origins, log,
		publish.New(writer, repo).WithReports(repo), writer,
		api.NewAdminKey(cfg.AdminKey, cfg.Org, cfg.Project))

	datasets, reports, schedules := repo.Counts()
	log.Info("cronosd listening",
		"addr", cfg.Addr, "driver", cfg.Driver,
		"datasets", datasets, "reports", reports, "schedules", schedules,
		"origins", cfg.Origins, "management", len(cfg.AdminKey) > 0,
		"scheduler", cfg.Scheduler,
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

func dialectFor(driver string) (query.Dialect, error) {
	switch driver {
	case "postgres", "pgx", "duckdb":
		return query.Postgres{}, nil
	case "sqlite":
		return query.SQLite{}, nil
	case "mysql":
		return query.MySQL{}, nil
	}
	// Guessing would produce statements that are subtly wrong rather than
	// statements that fail — a placeholder style that happens to parse binds
	// the wrong values.
	return nil, errors.New("cronosd: no dialect for driver " + driver)
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
