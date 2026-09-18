package sql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Which projects somebody belongs to.

An account holds one org and one project — where it was created, and where a
session starts. This is everywhere else it may go. The two are kept apart
deliberately: the account row is what every sign-in reads, and a migration that
rewrites it has to be right the first time.

Granted out of band, like an organization role, and never by anything the
holder sends. A membership decides what somebody may do in a project, so a
claim that could add one would be a claim that admits its own bearer.
*/

// projectRoles are the grants this store will record. A closed set, for the
// same reason the org roles are: query compilation reads these by name, and a
// fourth spelling would be a grant that silently does nothing.
var projectRoles = map[principal.Role]bool{
	principal.ProjectAdmin:  true,
	principal.ProjectEditor: true,
	principal.ProjectViewer: true,
}

// ErrNoProjectRole is what an unknown role is refused with.
var ErrNoProjectRole = errors.New("sql: not a project role")

/*
GrantMembership records that somebody belongs to a project.

Idempotent on the key, so granting twice changes the role rather than failing —
an administrator correcting a viewer to an editor is doing the same thing they
did the first time, and a conflict there would read as "already done" when it
is not.
*/
func (s *Store) GrantMembership(ctx context.Context, userID, org, project, role, by string) error {
	if !projectRoles[principal.Role(role)] {
		return ErrNoProjectRole
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT COUNT(*) FROM cronos_users WHERE id = ?`), userID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return identity.ErrNoUser
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, s.sql(`
		UPDATE cronos_memberships SET role = ?, granted_at = ?, granted_by = ?
		WHERE user_id = ? AND org = ? AND project = ?`),
		role, now, by, userID, org, project)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		return nil
	}
	_, err = s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_memberships (user_id, org, project, role, granted_at, granted_by)
		VALUES (?, ?, ?, ?, ?, ?)`), userID, org, project, role, now, by)
	return err
}

/*
RevokeMembership takes a project away.

The account is untouched. Somebody who was added to a second project and then
removed from it is still the account they were, in the project they started in
— deleting the row here and the account there are different decisions, and one
of them loses the answer to "who ran this in March".
*/
func (s *Store) RevokeMembership(ctx context.Context, userID, org, project string) error {
	res, err := s.db.ExecContext(ctx, s.sql(`
		DELETE FROM cronos_memberships WHERE user_id = ? AND org = ? AND project = ?`),
		userID, org, project)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return identity.ErrNoUser
	}
	return nil
}

// MembershipsOf is every project somebody may enter, oldest grant first so the
// list does not reorder itself between two reads.
func (s *Store) MembershipsOf(ctx context.Context, userID string) ([]identity.Membership, error) {
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT org, project, role FROM cronos_memberships
		WHERE user_id = ? ORDER BY granted_at, org, project`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []identity.Membership
	for rows.Next() {
		var m identity.Membership
		if err := rows.Scan(&m.Org, &m.Project, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

/*
MembershipIn is the role somebody holds in one project, and whether they hold
one at all.

Asked as its own question rather than filtered out of MembershipsOf, because
the caller that needs it is deciding whether to admit somebody — and a decision
that walks a list is one that reads the whole list into memory first, on a path
that runs per sign-in.
*/
func (s *Store) MembershipIn(ctx context.Context, userID, org, project string) (string, bool, error) {
	var role string
	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT role FROM cronos_memberships
		WHERE user_id = ? AND org = ? AND project = ?`), userID, org, project).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return role, true, nil
}
