package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/gsoultan/cronos/internal/adapter/store/file"
	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/platform/config"
)

// definitionStore chooses where published definitions live.
//
// A directory by default, because a definitions repository is a git repository
// and a publish is a commit somebody can review. A database when one is
// configured, because that is what makes management multi-tenant: the file
// store holds one project and says so rather than serving it to whoever asks.
func definitionStore(ctx context.Context, cfg config.Server, repo *file.Repository,
	log *slog.Logger) (publish.Store, func() error, error) {

	if cfg.StoreDSN == "" {
		log.Info("definition store", "kind", "files", "dir", cfg.Definitions,
			"project", cfg.Org+"/"+cfg.Project)
		return file.NewWriter(cfg.Definitions, cfg.Org, cfg.Project, repo), func() error { return nil }, nil
	}

	db, err := sql.Open(cfg.StoreDriver, cfg.StoreDSN)
	if err != nil {
		return nil, nil, err
	}
	mark, err := placeholders(cfg.StoreDriver)
	if err != nil {
		db.Close()
		return nil, nil, err
	}

	store := sqlstore.New(db, mark)
	if err := store.Migrate(ctx); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("definition store: %w", err)
	}
	log.Info("definition store", "kind", "database", "driver", cfg.StoreDriver)
	return store, db.Close, nil
}

// placeholders says how the driver marks a bind argument.
//
// Chosen rather than sniffed: a caller already decided which database this is
// in order to open it, and guessing would be a second place for that decision
// to be made differently.
func placeholders(driver string) (func(int) string, error) {
	switch driver {
	case "postgres", "pgx":
		return sqlstore.Dollar, nil
	case "sqlite", "mysql":
		return sqlstore.Question, nil
	}
	return nil, fmt.Errorf("definition store: no placeholder style for driver %q", driver)
}
