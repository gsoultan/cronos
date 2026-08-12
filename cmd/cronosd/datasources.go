package main

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/gsoultan/cronos/internal/adapter/driver/registry"
	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/adapter/store/file"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
	"github.com/gsoultan/cronos/internal/platform/config"
)

// datasources decides where a dataset's rows come from.
//
// The registry when the repository defines datasources, which is the real
// deployment: a dataset naming a warehouse reaches that warehouse, with that
// warehouse's own timeout and row cap.
//
// The configured CRONOS_DSN when it does not. That is the development path —
// one seeded database, every dataset reading it — and it stays because a demo
// that needs four YAML files before it shows a number is a demo nobody runs.
func datasources(cfg config.Server, repo *file.Repository,
	log *slog.Logger) (run.Engines, func() error, error) {

	if defs := repo.DataSources(); len(defs) > 0 {
		reg, err := registry.New(defs, secrets(cfg), log)
		if err != nil {
			return nil, nil, err
		}
		if err := seedRegistry(cfg, reg, log); err != nil {
			reg.Close()
			return nil, nil, err
		}
		log.Info("datasources", "kind", "defined", "names", reg.Names())
		return reg, reg.Close, nil
	}

	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, nil, err
	}
	if err := seed(db, cfg.Seed, log); err != nil {
		db.Close()
		return nil, nil, err
	}
	dialect, err := dialectFor(cfg.Driver)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	log.Info("datasources", "kind", "configured", "driver", cfg.Driver)

	// One engine for everything, said out loud rather than assumed: this is a
	// deployment that reads a single database, and a dataset naming a source
	// it has never heard of still resolves here.
	return run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db).WithLimits(definition.Limits{}),
		Builder:  query.NewBuilder(dialect),
	}}, db.Close, nil
}

// dialectFor maps the configured driver to the SQL it speaks.
//
// Guessing would produce statements that are subtly wrong rather than
// statements that fail: a placeholder style that happens to parse binds the
// wrong values to the right-looking query.
func dialectFor(driver string) (query.Dialect, error) {
	switch driver {
	case "postgres", "pgx", "duckdb":
		return query.Postgres{}, nil
	case "sqlite":
		return query.SQLite{}, nil
	case "mysql":
		return query.MySQL{}, nil
	}
	return nil, fmt.Errorf("cronosd: no dialect for driver %q", driver)
}

// seedRegistry applies a development seed to a defined datasource.
//
// Which one has to be unambiguous. cronos does not own the databases it reads,
// and running DDL against the wrong warehouse because a flag was set is not a
// mistake anybody should be able to make by leaving a variable unset.
func seedRegistry(cfg config.Server, reg *registry.Registry, log *slog.Logger) error {
	if cfg.Seed == "" {
		return nil
	}
	name := cfg.SeedSource
	if name == "" {
		only, ok := reg.Only()
		if !ok {
			return fmt.Errorf("cronosd: CRONOS_SEED needs CRONOS_SEED_SOURCE when several " +
				"datasources are defined — it runs DDL, and picking one would be a guess")
		}
		name = only
	}
	db, ok := reg.DB(name)
	if !ok {
		return fmt.Errorf("cronosd: cannot seed %q — no such datasource", name)
	}
	return seed(db, cfg.Seed, log)
}
