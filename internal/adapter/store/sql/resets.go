package sql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
StartReset records a reset somebody asked for.

Written before the mail goes out, the same order as an invitation: a link that
arrives before the row exists is a link that fails, and the person clicking it
learns only that this product is broken. The other way round leaves a row nobody
uses, which expires in an hour.
*/
func (s *Store) StartReset(ctx context.Context, r identity.Reset, hash string) error {
	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_password_resets
			(id, user_id, secret_hash, email, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`),
		r.ID, r.UserID, hash, r.Email, stamp(r.CreatedAt), stamp(r.Expires))
	return err
}

/*
RecentResets counts how many were asked for lately, for one account.

The rate limiter in front of the endpoint is per address and per IP and is
about noise. This is about the mailbox: somebody who holds down the button on
the sign-in page sends the account's owner a hundred emails, and the owner
cannot tell that from an attack. Counting per account is the only way to stop
that, because it is the only thing the flood has in common.
*/
func (s *Store) RecentResets(ctx context.Context, userID string, since time.Duration) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT COUNT(*) FROM cronos_password_resets
		WHERE user_id = ? AND created_at > ?`),
		userID, stamp(s.now().Add(-since))).Scan(&n)
	return n, err
}

/*
CompleteReset sets a new password, once, and ends every session that account has.

Three things in one transaction, and all three are load-bearing.

The UPDATE that spends the link carries the whole check — unspent, unexpired,
this exact secret — so the database decides rather than a SELECT this code ran a
moment ago. Two clicks arriving together both see an unspent link if it is read
first; only one of them gets a row from this.

The password is set from the row's own user_id, never from anything in the
request, so a redemption cannot be pointed at another account.

And the sessions are cut. "I cannot get in" and "somebody else is in" are the
same sentence from outside, so a reset that leaves the intruder's session
working has recovered nothing — and the token carries its own claims for up to
eight hours, so nothing else would end it.
*/
func (s *Store) CompleteReset(ctx context.Context, secret, password string) (identity.User, error) {
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
	secretHash := identity.HashReset(secret)

	spent, err := tx.ExecContext(ctx, s.sql(`
		UPDATE cronos_password_resets SET used_at = ?
		WHERE secret_hash = ? AND used_at IS NULL AND expires_at > ?`),
		stamp(now), secretHash, stamp(now))
	if err != nil {
		return identity.User{}, err
	}
	if n, err := spent.RowsAffected(); err != nil {
		return identity.User{}, err
	} else if n == 0 {
		return identity.User{}, identity.ErrReset
	}

	var userID string
	if err := tx.QueryRowContext(ctx, s.sql(`
		SELECT user_id FROM cronos_password_resets WHERE secret_hash = ?`), secretHash).
		Scan(&userID); err != nil {
		return identity.User{}, err
	}

	// Disabled accounts are refused here rather than when the link is asked
	// for. Somebody disabled between asking and clicking must not be let back
	// in by a link that was valid when it was sent — and the answer is the same
	// ErrReset as an expired one, so a held string never reveals whose it was.
	var u identity.User
	var created string
	if err := tx.QueryRowContext(ctx, s.sql(`
		SELECT id, email, name, org, project, role, created_at, disabled
		FROM cronos_users WHERE id = ?`), userID).
		Scan(&u.ID, &u.Email, &u.Name, &u.Org, &u.Project, &u.Role,
			&created, &u.Disabled); err != nil {

		if errors.Is(err, sql.ErrNoRows) {
			return identity.User{}, identity.ErrReset
		}
		return identity.User{}, err
	}
	if u.Disabled {
		return identity.User{}, identity.ErrReset
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, created)

	if _, err := tx.ExecContext(ctx, s.sql(`
		UPDATE cronos_users SET password = ? WHERE id = ?`), hash, userID); err != nil {
		return identity.User{}, err
	}

	if _, err := tx.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_sessions_cut (user_id, at) VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET at = EXCLUDED.at`),
		userID, stamp(now)); err != nil {
		return identity.User{}, err
	}

	/*
	   Every other outstanding link for this account is spent too.

	   Asking twice and clicking the first is ordinary — the second email is
	   still sitting in the mailbox, and it would otherwise still work. It is
	   also the shape of an attack that has read one email: ask again, wait for
	   the owner to reset, then use the older link.
	*/
	if _, err := tx.ExecContext(ctx, s.sql(`
		UPDATE cronos_password_resets SET used_at = ?
		WHERE user_id = ? AND used_at IS NULL`), stamp(now), userID); err != nil {
		return identity.User{}, err
	}

	if err := tx.Commit(); err != nil {
		return identity.User{}, err
	}
	return u, nil
}

/*
PruneResets drops spent and expired links.

They are single-use and short-lived, so the table is small; it is cleared anyway
because a table of hashes tied to addresses is one more thing a backup carries
around that nobody chose to keep.

`keep` is the grace after a link is finished with, and the comparison includes
the boundary so that a grace of zero means "everything already done with"
rather than "everything done with strictly before this instant" — which, with a
clock that does not move between the two, is nothing at all.
*/
func (s *Store) PruneResets(ctx context.Context, keep time.Duration) (int64, error) {
	cut := stamp(s.now().Add(-keep))
	res, err := s.db.ExecContext(ctx, s.sql(`
		DELETE FROM cronos_password_resets
		WHERE (used_at IS NOT NULL AND used_at <= ?) OR expires_at <= ?`), cut, cut)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
