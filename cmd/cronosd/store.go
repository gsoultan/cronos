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
	log *slog.Logger) (publish.Store, *sqlstore.Store, func() error, error) {

	if cfg.StoreDSN == "" {
		// No database, no run history. Said at startup rather than discovered
		// by an auditor asking a question nothing can answer.
		log.Info("definition store", "kind", "files", "dir", cfg.Definitions,
			"project", cfg.Org+"/"+cfg.Project, "history", false)
		return file.NewWriter(cfg.Definitions, cfg.Org, cfg.Project, repo),
			nil, func() error { return nil }, nil
	}

	// pgx registers itself as "pgx". An operator writing "postgres" means the
	// same thing and should not have to know which package was imported.
	driver := cfg.StoreDriver
	if driver == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, cfg.StoreDSN)
	if err != nil {
		return nil, nil, nil, err
	}
	mark, err := placeholders(cfg.StoreDriver)
	if err != nil {
		db.Close()
		return nil, nil, nil, err
	}

	if err := tune(ctx, db, cfg.StoreDriver); err != nil {
		db.Close()
		return nil, nil, nil, err
	}

	store := sqlstore.New(db, mark).ForDriver(cfg.StoreDriver)
	if err := store.Migrate(ctx); err != nil {
		db.Close()
		return nil, nil, nil, fmt.Errorf("definition store: %w", err)
	}
	log.Info("definition store", "kind", "database", "driver", cfg.StoreDriver, "history", true)
	return store, store, db.Close, nil
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

// tune applies the settings a driver needs to survive a burst.
//
// SQLite has one writer, and a burst has eight: without a busy timeout the
// concurrent delivery records fail immediately with SQLITE_BUSY, which is how
// a run came back saying it delivered to three customers and recorded none of
// them. WAL lets the listing endpoint read while those writes are happening.
//
// Postgres needs none of this and is left alone; guessing at pool sizes for
// somebody else's database is how a reporting tool becomes the reason their
// connection limit is reached.
func tune(ctx context.Context, db *sql.DB, driver string) error {
	if driver != "sqlite" {
		return nil
	}
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("definition store: %s: %w", pragma, err)
		}
	}
	return nil
}
