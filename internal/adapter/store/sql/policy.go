package sql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
What a project requires of the people in it.

One switch so far, and it is the one that needed a plan rather than a column:
requiring a second factor of everybody. The flag is trivial; what took deciding
is that somebody who has none cannot enrol without signing in and cannot sign in
without enrolling, so turning it on either locks a team out of its own reporting
or does nothing at all.

The answer here is neither. They sign in, to a session that reaches the enrolment
endpoints and nothing else — see api.Enrolment. Nobody is locked out and the
requirement bites on the next sign-in rather than on a deadline somebody misses
while on leave.
*/

// Policy is what a project requires.
type Policy struct {
	RequireTwoFactor bool `json:"requireTwoFactor"`
}

// PolicyOf reads it, defaulting to requiring nothing.
//
// A project with no row has no policy, which is every project until somebody
// sets one. Absence means "not required" rather than an error, because the
// alternative is a sign-in path that fails when a table is empty.
func (s *Store) PolicyOf(ctx context.Context, org, project string) (Policy, error) {
	var p Policy
	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT require_two_factor FROM cronos_policies
		WHERE org = ? AND project = ?`), org, project).Scan(&p.RequireTwoFactor)

	if errors.Is(err, sql.ErrNoRows) {
		return Policy{}, nil
	}
	return p, err
}

// SetPolicy records what a project requires.
//
// Scoped to the caller's own project, like every other administrative write:
// requiring a second factor of another organisation's people is not a thing a
// project administrator does, and a platform administrator does it by being a
// member there.
func (s *Store) SetPolicy(ctx context.Context, pr principal.Principal, p Policy) error {
	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_policies (org, project, require_two_factor, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (org, project) DO UPDATE SET
		  require_two_factor = EXCLUDED.require_two_factor,
		  updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by`),
		pr.OrgID, pr.ProjectID, p.RequireTwoFactor, stamp(s.now()), pr.Subject)
	return err
}

/*
Covered is how many people in a project have a second factor, and how many do not.

For the panel that turns the requirement on. Enabling blind is how a team finds
out on a Friday; showing the count first is the difference between a decision and
a surprise — and with the enrolment-only session nobody is locked out either
way, so this is information rather than a warning.
*/
func (s *Store) Covered(ctx context.Context, pr principal.Principal) (with, without int, err error) {
	/*
	   COALESCE, because SUM over no rows is NULL rather than zero.

	   On both drivers, and it scans into an int as "converting NULL to int is
	   unsupported" — a 500 from the panel that decides whether to require a
	   second factor. Reachable two ways: a project whose people have all been
	   turned off, and a machine credential, which carries an organisation and a
	   project and has no account row anywhere.

	   COUNT(*) would not need this and cannot express "how many of them have
	   one", which is the question.
	*/
	err = s.db.QueryRowContext(ctx, s.sql(`
		SELECT
		  COALESCE(SUM(CASE WHEN f.confirmed_at IS NOT NULL THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN f.confirmed_at IS NULL THEN 1 ELSE 0 END), 0)
		FROM cronos_users u
		LEFT JOIN cronos_factors f ON f.user_id = u.id
		WHERE u.org = ? AND u.project = ? AND NOT u.disabled`),
		pr.OrgID, pr.ProjectID).Scan(&with, &without)
	return with, without, err
}
