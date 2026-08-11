package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/history"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Begin records a run that has started.
func (s *Store) Begin(ctx context.Context, r history.Run) error {
	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_runs
			(id, org, project, schedule, report, report_version, output,
			 period_start, period_end, triggered_by, started_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		r.ID, r.Org, r.Project, r.Schedule, r.Report, r.ReportVersion, r.Output,
		r.PeriodStart, r.PeriodEnd, r.TriggeredBy, stamp(r.StartedAt), r.Status)
	return err
}

// Delivered records one document arriving somewhere, or not.
//
// Upserted on (run, recipient, channel): a retry is the same delivery having
// another go, not a second one, and two rows would make a burst look like it
// sent twice.
func (s *Store) Delivered(ctx context.Context, d history.Delivery) error {
	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_deliveries
			(run_id, recipient, channel, destination, filename, status, attempts, bytes, error, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (run_id, recipient, channel) DO UPDATE SET
			status = excluded.status, attempts = excluded.attempts,
			bytes = excluded.bytes, error = excluded.error, at = excluded.at`),
		d.RunID, d.Recipient, d.Channel, d.Destination, d.Filename,
		d.Status, d.Attempts, d.Bytes, d.Error, stamp(d.At))
	return err
}

// Finish closes a run.
func (s *Store) Finish(ctx context.Context, id string, r history.Run) error {
	_, err := s.db.ExecContext(ctx, s.sql(`
		UPDATE cronos_runs SET
			finished_at = ?, recipients = ?, delivered = ?, status = ?, error = ?
		WHERE id = ?`),
		stampp(r.FinishedAt), r.Recipients, r.Delivered, r.Status, r.Error, id)
	return err
}

// Runs lists a project's runs, newest first.
func (s *Store) Runs(ctx context.Context, pr principal.Principal, limit int) ([]history.Run, error) {
	if err := tenant(pr); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		// Capped, because the first thing anyone does with an unbounded
		// listing endpoint is call it on a table with a year of bursts in it.
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT id, org, project, schedule, report, report_version, output,
		       period_start, period_end, triggered_by, started_at, finished_at,
		       recipients, delivered, status, error
		FROM cronos_runs
		WHERE org = ? AND project = ?
		ORDER BY started_at DESC, id DESC
		LIMIT `+fmt.Sprint(limit)), pr.OrgID, pr.ProjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []history.Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Run returns one run and everything it delivered.
func (s *Store) Run(ctx context.Context, pr principal.Principal, id string) (history.Run, []history.Delivery, error) {
	if err := tenant(pr); err != nil {
		return history.Run{}, nil, err
	}
	// The tenant is in the predicate even though the id is unique: an id from
	// another project must read as absent rather than as forbidden, or the
	// difference tells a caller it exists.
	row := s.db.QueryRowContext(ctx, s.sql(`
		SELECT id, org, project, schedule, report, report_version, output,
		       period_start, period_end, triggered_by, started_at, finished_at,
		       recipients, delivered, status, error
		FROM cronos_runs WHERE id = ? AND org = ? AND project = ?`),
		id, pr.OrgID, pr.ProjectID)

	run, err := scanRun(row)
	if err != nil {
		return history.Run{}, nil, fmt.Errorf("%w: run %q", publish.ErrNotFound, id)
	}

	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT run_id, recipient, channel, destination, filename,
		       status, attempts, bytes, error, at
		FROM cronos_deliveries WHERE run_id = ? ORDER BY recipient, channel`), id)
	if err != nil {
		return run, nil, err
	}
	defer rows.Close()

	var out []history.Delivery
	for rows.Next() {
		var d history.Delivery
		var at string
		if err := rows.Scan(&d.RunID, &d.Recipient, &d.Channel, &d.Destination,
			&d.Filename, &d.Status, &d.Attempts, &d.Bytes, &d.Error, &at); err != nil {
			return run, nil, err
		}
		d.At, _ = time.Parse(time.RFC3339, at)
		out = append(out, d)
	}
	return run, out, rows.Err()
}

// scanner is what both QueryRow and Rows satisfy.
type scanner interface{ Scan(...any) error }

func scanRun(row scanner) (history.Run, error) {
	var r history.Run
	var started string
	var finished sql.NullString

	err := row.Scan(&r.ID, &r.Org, &r.Project, &r.Schedule, &r.Report, &r.ReportVersion,
		&r.Output, &r.PeriodStart, &r.PeriodEnd, &r.TriggeredBy, &started, &finished,
		&r.Recipients, &r.Delivered, &r.Status, &r.Error)
	if err != nil {
		return history.Run{}, err
	}
	r.StartedAt, _ = time.Parse(time.RFC3339, started)
	if finished.Valid && finished.String != "" {
		if t, err := time.Parse(time.RFC3339, finished.String); err == nil {
			r.FinishedAt = &t
		}
	}
	return r, nil
}

// stamp writes a timestamp that sorts as a string.
//
// RFC 3339 in UTC, so ORDER BY started_at is chronological on every database
// this runs on — a local-time or a driver-specific timestamp type would sort
// differently on Postgres and SQLite, and the listing is the whole feature.
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func stampp(t *time.Time) any {
	if t == nil {
		return nil
	}
	return stamp(*t)
}
