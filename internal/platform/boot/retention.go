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

/*
KeepAcceptedInvitations is how long a used invitation's row survives.

Thirty days. After that its only remaining value is telling an auditor that this
account arrived by invitation, and the audit log already says that in a place
built for it. What is left here is an email address in a table with no purpose,
which is the definition of data kept by accident.

Unusable ones go as soon as they expire, because they are already dead.
*/
const KeepAcceptedInvitations = 30 * 24 * time.Hour

/*
sweepInvitations removes the ones nobody can use.

Its own loop rather than a line inside retain(), because it runs whatever the
history retention is set to. Run history is a deliberate choice about how much
to keep; an expired invitation is not kept on purpose by anybody.

Hourly rather than daily: the row is small, but it holds an address and a hash
of a credential, and there is no reason for either to outlive the week the link
was good for by more than an hour.
*/
func sweepInvitations(ctx context.Context, records *sqlstore.Store, log *slog.Logger) {
	if records == nil {
		return
	}

	sweep := func() {
		gone, err := records.PruneInvitations(ctx, KeepAcceptedInvitations)
		if err != nil {
			log.Error("pruning invitations", "err", err)
			return
		}
		if gone > 0 {
			log.Info("pruned invitations", "rows", gone)
		}

		/*
		   Password resets go with them, and sooner.

		   A link is good for an hour and is spent the first time it is used, so
		   there is nothing left to keep by the time this runs. An hour's grace
		   past that, rather than none, so a row is never deleted out from under
		   a request still holding it.

		   Kept in the same sweep because it is the same kind of thing: a hash
		   of a credential tied to an address, sitting in a table for no reason
		   anybody chose.
		*/
		gone, err = records.PruneResets(ctx, time.Hour)
		if err != nil {
			log.Error("pruning password resets", "err", err)
			return
		}
		if gone > 0 {
			log.Info("pruned password resets", "rows", gone)
		}
	}

	go func() {
		sweep()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweep()
			}
		}
	}()
}
