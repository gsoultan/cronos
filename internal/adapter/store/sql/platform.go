package sql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
Platform administration: the tier above organisations.

Everything else in this store is scoped by the caller's organisation and project,
because that is what keeps one customer out of another's. These queries
deliberately are not, and that is exactly why each one is only reachable from a
handler that has checked CanAdminPlatform first — the scoping that protects
every other read is absent here by design, so the check has to be somewhere, and
"somewhere" needs to be obvious.

What is *not* here is any way to read a project's data. A platform administrator
adds accounts, moves people between projects and sees which tenants a process
serves. Opening a report still requires membership. That boundary is the reason
a leaked platform credential is a control-plane problem rather than every
customer's data at once, and it is kept by there being no function below that
returns a row of anybody's warehouse.
*/

// EveryPerson lists all accounts, in every organisation and project.
//
// The one unscoped read of the roster. Ordered by where they are so the answer
// groups by tenant without the caller sorting it.
func (s *Store) EveryPerson(ctx context.Context) ([]identity.User, error) {
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT u.id, u.email, u.name, u.org, u.project, u.role,
		       u.created_at, u.last_seen, u.disabled
		FROM cronos_users u
		ORDER BY u.org, u.project, u.email`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []identity.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Tenants lists the organisations and projects that have accounts.
//
// From the roster rather than from configuration: what a process was told to
// serve is boot's business, and this answers the different question of where
// people actually are — including a project nobody is serving any more, which
// is worth seeing rather than hiding.
func (s *Store) Tenants(ctx context.Context) ([]identity.Tenant, error) {
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT org, project, COUNT(*), SUM(CASE WHEN disabled THEN 1 ELSE 0 END)
		FROM cronos_users
		GROUP BY org, project
		ORDER BY org, project`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []identity.Tenant{}
	for rows.Next() {
		var t identity.Tenant
		if err := rows.Scan(&t.Org, &t.Project, &t.People, &t.Disabled); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MovePerson changes where somebody works, and what they are there.
//
// Unscoped, so it can move an account from one organisation to another — the
// thing an ordinary administrator must never be able to do, and the reason this
// file exists.
func (s *Store) MovePerson(ctx context.Context, id, org, project, role string) error {
	if org == "" || project == "" {
		return fmt.Errorf("%w: an account needs an organisation and a project",
			identity.ErrBadCredentials)
	}
	out, err := s.db.ExecContext(ctx, s.sql(`
		UPDATE cronos_users SET org = ?, project = ?, role = ? WHERE id = ?`),
		org, project, role, id)
	if err != nil {
		return err
	}
	if n, err := out.RowsAffected(); err == nil && n == 0 {
		return identity.ErrNoUser
	}
	return nil
}

// DisableAnywhere turns access off whatever project somebody is in.
func (s *Store) DisableAnywhere(ctx context.Context, id string, disabled bool) error {
	out, err := s.db.ExecContext(ctx, s.sql(
		`UPDATE cronos_users SET disabled = ? WHERE id = ?`), disabled, id)
	if err != nil {
		return err
	}
	if n, err := out.RowsAffected(); err == nil && n == 0 {
		return identity.ErrNoUser
	}
	return nil
}

// IsPlatformAdmin reports whether this account administers the deployment.
func (s *Store) IsPlatformAdmin(ctx context.Context, id string) bool {
	var at string
	err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT granted_at FROM cronos_platform_admins WHERE user_id = ?`), id).Scan(&at)
	return err == nil
}

// PlatformAdmins lists them, with the accounts they belong to.
func (s *Store) PlatformAdmins(ctx context.Context) ([]identity.User, error) {
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT u.id, u.email, u.name, u.org, u.project, u.role,
		       u.created_at, u.last_seen, u.disabled
		FROM cronos_platform_admins a
		JOIN cronos_users u ON u.id = a.user_id
		ORDER BY u.email`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []identity.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		u.Platform = true
		out = append(out, u)
	}
	return out, rows.Err()
}

// GrantPlatform makes somebody a platform administrator.
//
// Idempotent, because "make them an administrator" is a statement about the
// end state and pressing it twice should not be an error somebody has to read.
func (s *Store) GrantPlatform(ctx context.Context, id, by string) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT COUNT(*) FROM cronos_users WHERE id = ?`), id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return identity.ErrNoUser
	}

	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_platform_admins (user_id, granted_at, granted_by)
		VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO NOTHING`),
		id, stamp(s.now()), by)
	return err
}

/*
RevokePlatform takes it away, and refuses to take away the last one.

A deployment with no platform administrator cannot make another: the endpoints
that grant one require the permission being granted. The only way back is the
CLI on the machine, which for a hosted deployment means a person with shell
access — so the guard is not paternalism, it is the difference between a mistake
and an outage.
*/
func (s *Store) RevokePlatform(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Counted inside the transaction, so two revocations racing cannot both
	// see two administrators and both proceed.
	var count int
	if err := tx.QueryRowContext(ctx, s.sql(
		`SELECT COUNT(*) FROM cronos_platform_admins`)).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastPlatformAdmin
	}

	out, err := tx.ExecContext(ctx, s.sql(
		`DELETE FROM cronos_platform_admins WHERE user_id = ?`), id)
	if err != nil {
		return err
	}
	if n, err := out.RowsAffected(); err == nil && n == 0 {
		return identity.ErrNoUser
	}

	/*
	   And their sessions end.

	   The token carries the platform claim so that every request does not have
	   to ask the database, which means a revoked administrator keeps the claim
	   until their token expires — up to eight hours. A revocation that takes
	   eight hours is not a revocation, and cutting their sessions is both
	   simpler than checking per request and a stronger answer: they are signed
	   out, rather than signed in with less.
	*/
	line := s.now().Truncate(time.Second).Add(time.Second)
	if _, err := tx.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_sessions_cut (user_id, at) VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET at = EXCLUDED.at`),
		id, stamp(line)); err != nil {
		return err
	}
	return tx.Commit()
}

// ErrLastPlatformAdmin is refusing to leave a deployment with none.
var ErrLastPlatformAdmin = errors.New("sql: this is the last platform administrator")

// CountAccounts is how many accounts exist at all, in any project.
//
// For the first-run check: zero is a deployment nobody can sign in to, which is
// the only state /v1/setup is open in.
func (s *Store) CountAccounts(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.sql(`SELECT COUNT(*) FROM cronos_users`)).Scan(&n)
	return n, err
}
