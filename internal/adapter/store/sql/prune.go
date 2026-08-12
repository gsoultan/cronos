package sql

import (
	"context"
	"fmt"
	"time"
)

/*
Prune removes run records older than a cutoff, and what they delivered.

Two tables grow without bound and nothing was removing from either. One
monthly schedule bursting to five thousand customers writes sixty thousand
delivery rows a year, and a deployment running ten of them for three years is
holding two million rows describing sends nobody will ask about again — on the
same database that answers "did last night work", which is the question that
gets slower.

Deliveries first, then the runs they belong to. The other order leaves rows
whose run has gone, which is a table that can only be cleaned by knowing what
used to be in another one.

Returns what it removed, so the line that reports it says a number rather than
"pruned".
*/
func (s *Store) Prune(ctx context.Context, before time.Time) (runs, deliveries int64, err error) {
	cutoff := stamp(before)

	// Bounded by a subquery rather than a join, because the two tables are
	// deleted from separately and a join would need the runs to still be
	// there for the second delete — which is the order that cannot work.
	result, err := s.db.ExecContext(ctx, s.sql(`
		DELETE FROM cronos_deliveries
		WHERE run_id IN (SELECT id FROM cronos_runs WHERE started_at < ?)`), cutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("sql: pruning deliveries: %w", err)
	}
	deliveries, _ = result.RowsAffected()

	result, err = s.db.ExecContext(ctx, s.sql(
		`DELETE FROM cronos_runs WHERE started_at < ?`), cutoff)
	if err != nil {
		// The deliveries are already gone. That is the safe half to have done:
		// a run with no delivery rows reads as a run whose detail has aged
		// out, and the next pass removes the run itself.
		return 0, deliveries, fmt.Errorf("sql: pruning runs: %w", err)
	}
	runs, _ = result.RowsAffected()

	return runs, deliveries, nil
}
