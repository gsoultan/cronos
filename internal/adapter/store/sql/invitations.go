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
Invitations, and the one place they must be atomic.

An invitation is a secret somebody holds and an account that does not exist yet.
Accepting it is two writes — create the person, spend the invitation — and doing
them separately has two failure modes, both bad: a crash between them leaves
either an account nobody can reach or a live invitation for an account that now
exists, which is a second way in that nobody issued.

So Accept is one transaction, and the spend is conditional on the invitation
still being unspent. Two browsers submitting the same link at the same moment is
not hypothetical — it is a double-click.
*/

// Invite records one. The secret is never stored; its hash is the lookup key.
func (s *Store) Invite(ctx context.Context, inv identity.Invitation, hash string) error {
	email := normalise(inv.Email)
	if email == "" {
		return fmt.Errorf("%w: no email", identity.ErrBadCredentials)
	}

	// Somebody who already has an account here is not invited again — the link
	// would create a second account for the same address, or fail confusingly
	// at the end after they had chosen a password.
	var exists int
	if err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT COUNT(*) FROM cronos_users WHERE email = ?`), email).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return fmt.Errorf("%w: %s", identity.ErrExists, email)
	}

	// Any earlier invitation for this address in this project is replaced. Two
	// live links to one account is one more than anybody meant to send, and the
	// common reason to invite somebody twice is that the first mail went
	// astray — in which case the first link should stop working.
	if _, err := s.db.ExecContext(ctx, s.sql(`
		DELETE FROM cronos_invitations
		WHERE email = ? AND org = ? AND project = ? AND accepted_at IS NULL`),
		email, inv.Org, inv.Project); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_invitations
		  (id, secret_hash, email, name, org, project, role, invited_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		inv.ID, hash, email, inv.Name, inv.Org, inv.Project, inv.Role,
		inv.InvitedBy, stamp(s.now()), stamp(inv.Expires))
	return err
}

// Invitation reads one by the secret somebody presented.
//
// By hash, so the secret in the request is never compared against anything
// stored in the clear, and the query itself carries no usable credential into a
// slow-query log.
func (s *Store) Invitation(ctx context.Context, secret string) (identity.Invitation, error) {
	var inv identity.Invitation
	var created, expires string
	var accepted sql.NullString

	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT id, email, name, org, project, role, invited_by, created_at, expires_at, accepted_at
		FROM cronos_invitations WHERE secret_hash = ?`), identity.HashInvitation(secret)).
		Scan(&inv.ID, &inv.Email, &inv.Name, &inv.Org, &inv.Project, &inv.Role,
			&inv.InvitedBy, &created, &expires, &accepted)

	if errors.Is(err, sql.ErrNoRows) {
		return identity.Invitation{}, identity.ErrInvitation
	}
	if err != nil {
		return identity.Invitation{}, err
	}

	inv.CreatedAt, inv.Expires = unstamp(created), unstamp(expires)
	if accepted.Valid {
		at := unstamp(accepted.String)
		inv.Accepted = &at
	}
	if !inv.Usable(s.now()) {
		// Expired or spent. The same error as one that never existed: an
		// endpoint with no session that distinguishes them is a way to learn
		// which invitations are outstanding.
		return identity.Invitation{}, identity.ErrInvitation
	}
	return inv, nil
}

/*
Accept turns an invitation into an account, once.

The UPDATE carries the whole check — unspent, unexpired, and this exact secret —
so the database decides, not a read this code did a moment ago. Two requests
arriving together both see an unspent invitation if it is checked with a SELECT;
only one of them gets a row from this UPDATE.
*/
func (s *Store) Accept(ctx context.Context, secret, password string) (identity.User, error) {
	hash, err := identity.Hash(password)
	if err != nil {
		return identity.User{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return identity.User{}, err
	}
	defer func() { _ = tx.Rollback() }()

	now := s.now()
	secretHash := identity.HashInvitation(secret)

	// Spend it first. If this matches nothing the invitation was already used,
	// has expired, or never existed, and no account is created.
	spent, err := tx.ExecContext(ctx, s.sql(`
		UPDATE cronos_invitations SET accepted_at = ?
		WHERE secret_hash = ? AND accepted_at IS NULL AND expires_at > ?`),
		stamp(now), secretHash, stamp(now))
	if err != nil {
		return identity.User{}, err
	}
	if n, err := spent.RowsAffected(); err != nil {
		return identity.User{}, err
	} else if n == 0 {
		return identity.User{}, identity.ErrInvitation
	}

	var inv identity.Invitation
	if err := tx.QueryRowContext(ctx, s.sql(`
		SELECT id, email, name, org, project, role
		FROM cronos_invitations WHERE secret_hash = ?`), secretHash).
		Scan(&inv.ID, &inv.Email, &inv.Name, &inv.Org, &inv.Project, &inv.Role); err != nil {
		return identity.User{}, err
	}

	user := identity.User{
		ID: identity.NewID(), Email: inv.Email, Name: inv.Name,
		Org: inv.Org, Project: inv.Project, Role: inv.Role, CreatedAt: now,
	}
	if _, err := tx.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_users (id, email, name, password, org, project, role, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		user.ID, user.Email, user.Name, hash, user.Org, user.Project, user.Role,
		stamp(now)); err != nil {

		if duplicate(err) {
			// An account for this address appeared between the invitation
			// being written and it being accepted. Rolling back leaves the
			// invitation unspent, which is right: nothing happened.
			return identity.User{}, fmt.Errorf("%w: %s", identity.ErrExists, user.Email)
		}
		return identity.User{}, err
	}

	if err := tx.Commit(); err != nil {
		return identity.User{}, err
	}
	return user, nil
}

// Invitations lists the ones still outstanding in the caller's project.
//
// So that "invited" is a state somebody can see. Without it an administrator
// who sent an invitation has no way to tell whether it arrived, was accepted,
// or expired quietly a week ago.
func (s *Store) Invitations(ctx context.Context, pr principal.Principal) ([]identity.Invitation, error) {
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT id, email, name, org, project, role, invited_by, created_at, expires_at
		FROM cronos_invitations
		WHERE org = ? AND project = ? AND accepted_at IS NULL AND expires_at > ?
		ORDER BY created_at DESC`),
		pr.OrgID, pr.ProjectID, stamp(s.now()))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []identity.Invitation{}
	for rows.Next() {
		var inv identity.Invitation
		var created, expires string
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Name, &inv.Org, &inv.Project,
			&inv.Role, &inv.InvitedBy, &created, &expires); err != nil {
			return nil, err
		}
		inv.CreatedAt, inv.Expires = unstamp(created), unstamp(expires)
		out = append(out, inv)
	}
	return out, rows.Err()
}

// Uninvite withdraws one before it is used.
//
// Somebody invited by mistake, or to an address that turned out to be wrong.
// Scoped to the caller's project, so an administrator of one cannot cancel
// another's.
func (s *Store) Uninvite(ctx context.Context, pr principal.Principal, id string) error {
	out, err := s.db.ExecContext(ctx, s.sql(`
		DELETE FROM cronos_invitations
		WHERE id = ? AND org = ? AND project = ? AND accepted_at IS NULL`),
		id, pr.OrgID, pr.ProjectID)
	if err != nil {
		return err
	}
	if n, err := out.RowsAffected(); err == nil && n == 0 {
		return identity.ErrInvitation
	}
	return nil
}

// PruneInvitations removes the ones nobody can use any more.
//
// Accepted ones too, after a while: the row's only remaining value is telling
// an auditor that this account arrived by invitation, and the audit log already
// says that. What is left here is an email address in a table with no purpose.
func (s *Store) PruneInvitations(ctx context.Context, keepAccepted time.Duration) (int64, error) {
	now := s.now()
	out, err := s.db.ExecContext(ctx, s.sql(`
		DELETE FROM cronos_invitations
		WHERE (accepted_at IS NULL AND expires_at < ?)
		   OR (accepted_at IS NOT NULL AND accepted_at < ?)`),
		stamp(now), stamp(now.Add(-keepAccepted)))
	if err != nil {
		return 0, err
	}
	return out.RowsAffected()
}

// unstamp reads back what stamp wrote.
//
// A zero time on anything unparseable, which for an expiry means "expired" and
// for a creation date means "unknown" — both of which fail closed, and neither
// of which is reachable from a row this code wrote.
func unstamp(s string) time.Time {
	at, _ := time.Parse(time.RFC3339, s)
	return at
}
