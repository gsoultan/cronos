package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Who administers an organization.

principal has had org roles from the start and nothing set one, so CanAdminOrg
was always false and the promotion in effective() never fired — while
docs/tenancy.md described what they do. This is where one comes from.

Granted out of band by a platform administrator and never by anything the
holder sends. An org role enters every project in the organization, so a claim
that could set it would be a claim that grants administration of everything the
organization owns.
*/

// orgRoles are the grants this store will record. A closed set, because the
// promotion in effective() reads two of them by name and a fourth spelling
// would be a grant that silently does nothing.
var orgRoles = map[principal.Role]bool{
	principal.OrgOwner:  true,
	principal.OrgAdmin:  true,
	principal.OrgMember: true,
}

// ErrNoOrgRole is what an unknown role is refused with.
var ErrNoOrgRole = errors.New("sql: not an organization role")

/*
GrantOrgRole records that somebody administers their organization.

The org is taken from the user's own row rather than from the caller. A
platform administrator granting across tenants is the point of the endpoint
that calls this, and letting it name the organization separately would let a
typo grant administration of somebody else's.

Idempotent on the pair, because "make them an org admin" is a statement about
the end state, and changing owner to admin is a correction rather than an
error.
*/
func (s *Store) GrantOrgRole(ctx context.Context, id string, role principal.Role, by string) error {
	if !orgRoles[role] {
		return fmt.Errorf("%w: %q", ErrNoOrgRole, role)
	}

	var org string
	switch err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT org FROM cronos_users WHERE id = ?`), id).Scan(&org); {
	case errors.Is(err, sql.ErrNoRows):
		return identity.ErrNoUser
	case err != nil:
		return err
	}

	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, s.sql(`
			INSERT INTO cronos_org_roles (org, user_id, role, granted_at, granted_by)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (org, user_id) DO UPDATE SET role = excluded.role,
				granted_at = excluded.granted_at, granted_by = excluded.granted_by`),
			org, id, string(role), stamp(s.now()), by); err != nil {
			return err
		}
		// Cut on the way up as well as down. Changing owner to member is a
		// reduction, and a session minted a minute earlier still carries the
		// larger role for the rest of the working day.
		return s.cutSessions(ctx, tx, id)
	})
}

/*
RevokeOrgRole takes it away, and ends that account's sessions.

Absent is success: the end state is the same, and an error here is one somebody
has to read to learn nothing.

The sessions are not optional. The role is in the token so that a request does
not ask the database for it, which means a revoked org admin keeps
administration of every project in the organization until the token expires —
up to eight hours. A revocation that takes eight hours is not a revocation.
*/
func (s *Store) RevokeOrgRole(ctx context.Context, id string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, s.sql(
			`DELETE FROM cronos_org_roles WHERE user_id = ?`), id); err != nil {
			return err
		}
		return s.cutSessions(ctx, tx, id)
	})
}

// inTx runs fn in a transaction, so a grant that cannot cut the sessions it
// invalidates is not a grant that happened.
func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// cutSessions draws the line every token issued before now falls behind.
func (s *Store) cutSessions(ctx context.Context, tx *sql.Tx, id string) error {
	line := s.now().Truncate(time.Second).Add(time.Second)
	_, err := tx.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_sessions_cut (user_id, at) VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET at = EXCLUDED.at`),
		id, stamp(line))
	return err
}

/*
OrgRoleOf is the role this account holds in its own organization.

principal.None when it holds none, which is every account in a deployment that
has never granted one — so this answers the same way for an upgrade as it did
before the table existed.

A role the database holds and this build does not recognise reads as None
rather than as an error. A row written by a later version is a grant this
version cannot honour, and failing the sign-in over it would take somebody's
access away for a spelling it will understand after an upgrade.
*/
func (s *Store) OrgRoleOf(ctx context.Context, id string) (principal.Role, error) {
	var role string
	switch err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT r.role FROM cronos_org_roles r
		JOIN cronos_users u ON u.id = r.user_id AND u.org = r.org
		WHERE r.user_id = ?`), id).Scan(&role); {
	case errors.Is(err, sql.ErrNoRows):
		return principal.None, nil
	case err != nil:
		return principal.None, err
	}
	if !orgRoles[principal.Role(role)] {
		return principal.None, nil
	}
	return principal.Role(role), nil
}

/*
ProjectsInOrg lists the projects an organization has.

Derived from the accounts in them rather than from configuration, because this
answers "where could an administrator go", and a project nobody belongs to is
one there is nothing to administer in. It is also the list that stays right
when a deployment's configuration and its data disagree.
*/
func (s *Store) ProjectsInOrg(ctx context.Context, org string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT DISTINCT project FROM cronos_users
		WHERE org = ? ORDER BY project`), org)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
