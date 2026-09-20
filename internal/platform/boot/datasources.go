package boot

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

/*
datasources decides where a dataset's rows come from.

The registry, always — a project holds however many sources it has been given,
and it is the same object whether they were read from a directory at startup or
published through the API this afternoon. A dataset naming a warehouse reaches
that warehouse, with that warehouse's own timeout and row cap.

The configured CRONOS_DSN answers while the registry is empty. That is the
development path — one seeded database, every dataset reading it — and it stays
because a demo that needs four YAML files before it shows a number is a demo
nobody runs. It stops the moment a source is defined, which is the rule that
keeps it from being a trap: a deployment with three warehouses must not answer a
dataset naming a fourth by quietly reading the development database instead.
*/
func datasources(cfg config.Server, repo *file.Repository,
	log *slog.Logger) (*sources, func() error, error) {

	reg, err := registry.New(repo.DataSources(), secrets(cfg), log)
	if err != nil {
		return nil, nil, err
	}

	if !reg.Empty() {
		if err := seedRegistry(cfg, reg, log); err != nil {
			reg.Close()
			return nil, nil, err
		}
		/*
		   A source this build cannot open is named, not fatal.

		   Reports that do not read it keep working; one that does fails with a
		   message naming the source. Loud, because a datasource that quietly
		   vanished is a report that returns an error nobody can trace back to a
		   definition — every reason at error, and counted for
		   cronos_datasources_unavailable, which should be zero.
		*/
		for _, why := range reg.Unavailable() {
			log.Error("datasource unavailable — reports that read it will fail", "err", why)
			unopenable.Add(1)
		}
		log.Info("datasources", "kind", "defined", "names", reg.Names())
		return &sources{registry: reg}, reg.Close, nil
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
	// it has never heard of still resolves here — until the day somebody
	// connects one, after which the registry above answers and this pool is
	// only held open so that shutting down closes it.
	configured := run.One{Only: run.Engine{
		Executor: sqldriver.NewExecutor(db).WithLimits(definition.Limits{}),
		Builder:  query.NewBuilder(dialect),
	}}
	return &sources{registry: reg, configured: configured}, func() error {
		reg.Close()
		return db.Close()
	}, nil
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
