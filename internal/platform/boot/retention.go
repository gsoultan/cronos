package boot

import (
	"context"
	"log/slog"
	"time"

	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/platform/config"
)

/*
retain removes run history older than the configured age, once a day.

Daily rather than on every write, because the cost of the delete is in finding
the rows and doing that per run turns a burst of five thousand into five
thousand scans. Daily rather than never, because the alternative is a table
that grows for the life of the deployment and is read by the one page somebody
opens when something has gone wrong.

Off by default. How long a business must keep evidence of what it sent its
customers is a legal question with a different answer in every jurisdiction it
operates in, and a default that quietly deleted at ninety days would be this
product answering it on their behalf.

Immediately at startup and then on a ticker: a deployment restarted daily
would otherwise never reach the first tick.
*/
func retain(ctx context.Context, records *sqlstore.Store, cfg config.Server, log *slog.Logger) {
	if records == nil || cfg.Retention <= 0 {
		return
	}
	log.Info("run history retention", "keep", cfg.Retention)

	prune := func() {
		runs, deliveries, err := records.Prune(ctx, time.Now().Add(-cfg.Retention))
		if err != nil {
			log.Error("pruning run history", "err", err)
			return
		}
		if runs > 0 || deliveries > 0 {
			log.Info("pruned run history",
				"runs", runs, "deliveries", deliveries, "older_than", cfg.Retention)
		}
	}

	go func() {
		prune()

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				prune()
			}
		}
	}()
}
