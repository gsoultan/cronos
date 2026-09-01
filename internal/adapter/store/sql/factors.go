package sql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
)

/*
Second factors, and the two places this can quietly stop being one.

The first is enrolment that is never proved. Offering a QR code and then trusting
that somebody scanned it leaves an account marked as protected by a secret nobody
has — which is worse than no second factor at all, because the person picks a
weaker password believing something else is guarding it. So a factor is not
confirmed until a code computed from that exact secret has been entered.

The second is replay. A TOTP code is valid for a whole thirty-second step, so
somebody who reads one off a shoulder, a shared screen or a screenshot can use it
again inside that window. The step is recorded and a repeat is refused, which is
what makes it one-time rather than thirty-seconds-long.
*/

// Enrol begins one, replacing any unconfirmed attempt.
//
// Replacing rather than refusing: somebody who started, lost the QR code and
// came back is the ordinary case, and a stuck half-enrolment they cannot clear
// is a support ticket. A *confirmed* factor is not replaced — turning one off
// is its own act, and doing it silently as a side effect of starting another
// would be a way to downgrade an account by asking politely.
func (s *Store) Enrol(ctx context.Context, id, secret, label string) error {
	var confirmed sql.NullString
	err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT confirmed_at FROM cronos_factors WHERE user_id = ?`), id).Scan(&confirmed)

	switch {
	case err == nil && confirmed.Valid:
		return identity.ErrFactorExists
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return err
	}

	_, err = s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_factors (user_id, secret, label, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
		  secret = EXCLUDED.secret, label = EXCLUDED.label,
		  created_at = EXCLUDED.created_at, confirmed_at = NULL, last_step = 0`),
		id, secret, label, stamp(s.now()))
	return err
}

// Enrolling returns the secret of an attempt in progress.
//
// Only while unconfirmed. Once a factor is proved, its secret never leaves the
// database again: an endpoint that hands it back is one that turns a stolen
// session into a permanent second factor of the attacker's own.
func (s *Store) Enrolling(ctx context.Context, id string) (string, error) {
	var secret string
	var confirmed sql.NullString

	err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT secret, confirmed_at FROM cronos_factors WHERE user_id = ?`), id).
		Scan(&secret, &confirmed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", identity.ErrNoFactor
	}
	if err != nil {
		return "", err
	}
	if confirmed.Valid {
		return "", identity.ErrFactorExists
	}
	return secret, nil
}

/*
Confirm proves the enrolment and turns protection on.

The code is checked against the secret that was stored, not against one supplied
with it — which sounds obvious and is the mistake that makes the whole thing
decorative: a check against a secret the caller sent is a check that the caller
can do arithmetic.
*/
func (s *Store) Confirm(ctx context.Context, id, code string) error {
	secret, err := s.Enrolling(ctx, id)
	if err != nil {
		return err
	}

	step, ok := identity.CheckTOTP(secret, code, s.now())
	if !ok {
		return identity.ErrBadCode
	}

	_, err = s.db.ExecContext(ctx, s.sql(`
		UPDATE cronos_factors SET confirmed_at = ?, last_step = ?
		WHERE user_id = ? AND confirmed_at IS NULL`),
		stamp(s.now()), step, id)
	return err
}

// Factor describes somebody's second factor, without its secret.
type Factor struct {
	Label     string    `json:"label"`
	AddedAt   time.Time `json:"addedAt"`
	Remaining int       `json:"remainingCodes"`
}

// FactorOf returns the confirmed factor, or ErrNoFactor.
//
// Never the secret. This is what the account page reads, and a field that
// carries the secret into a JSON response is one somebody adds without noticing
// what it is.
func (s *Store) FactorOf(ctx context.Context, id string) (Factor, error) {
	var f Factor
	var confirmed sql.NullString

	err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT label, confirmed_at FROM cronos_factors WHERE user_id = ?`), id).
		Scan(&f.Label, &confirmed)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !confirmed.Valid) {
		return Factor{}, identity.ErrNoFactor
	}
	if err != nil {
		return Factor{}, err
	}
	f.AddedAt = unstamp(confirmed.String)

	if err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT COUNT(*) FROM cronos_recovery_codes WHERE user_id = ?`), id).
		Scan(&f.Remaining); err != nil {
		return Factor{}, err
	}
	return f, nil
}

// Protected reports whether this account needs a second factor to sign in.
//
// Its own query because it runs on the sign-in path, before anybody is
// authenticated, and it must not depend on reading anything else about them.
func (s *Store) Protected(ctx context.Context, id string) bool {
	var confirmed sql.NullString
	err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT confirmed_at FROM cronos_factors WHERE user_id = ?`), id).Scan(&confirmed)
	return err == nil && confirmed.Valid
}

/*
CheckFactor verifies a code at sign-in and spends its step.

The spend is the whole point and it is conditional inside the UPDATE, not
decided by a read. Two sign-ins racing with the same stolen code both pass a
SELECT that says "this step is newer than the last"; only one gets a row from a
write that says the same thing.
*/
func (s *Store) CheckFactor(ctx context.Context, id, code string) error {
	var secret string
	var confirmed sql.NullString
	var last int64

	err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT secret, confirmed_at, last_step FROM cronos_factors WHERE user_id = ?`), id).
		Scan(&secret, &confirmed, &last)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !confirmed.Valid) {
		return identity.ErrNoFactor
	}
	if err != nil {
		return err
	}

	step, ok := identity.CheckTOTP(secret, code, s.now())
	if !ok {
		return identity.ErrBadCode
	}
	if step <= last {
		/*
		   Already used. A code is good for thirty seconds, which is long
		   enough to be read off a screen and typed somewhere else.

		   Redundant, strictly: the UPDATE below carries the same condition and
		   is the one that has to, because two requests racing both pass a
		   check made up here. Kept because it answers the common case — a
		   person double-submitting a form — without a write, and because the
		   rule reads better stated in Go than inferred from a WHERE clause.
		*/
		return identity.ErrCodeUsed
	}

	out, err := s.db.ExecContext(ctx, s.sql(`
		UPDATE cronos_factors SET last_step = ? WHERE user_id = ? AND last_step < ?`),
		step, id, step)
	if err != nil {
		return err
	}
	if n, err := out.RowsAffected(); err == nil && n == 0 {
		// Somebody else spent this step between the read and the write.
		return identity.ErrCodeUsed
	}
	return nil
}

/*
SetRecoveryCodes replaces the set, all or none.

In one transaction because the window between deleting the old and writing the
new is a window in which somebody has a second factor and no way back from a
lost phone. Regenerating invalidates the old set, which is the point: a sheet of
paper that has been photographed is replaced by asking for new ones.
*/
func (s *Store) SetRecoveryCodes(ctx context.Context, id string, hashes []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, s.sql(
		`DELETE FROM cronos_recovery_codes WHERE user_id = ?`), id); err != nil {
		return err
	}
	for _, hash := range hashes {
		if _, err := tx.ExecContext(ctx, s.sql(
			`INSERT INTO cronos_recovery_codes (user_id, code_hash) VALUES (?, ?)`),
			id, hash); err != nil {
			return err
		}
	}
	return tx.Commit()
}

/*
SpendRecoveryCode uses one, once.

The DELETE is the check: it either removes a row or it does not, and there is no
moment between deciding and doing in which a second request can decide the same
thing. A SELECT-then-DELETE would let one code let two people in.
*/
func (s *Store) SpendRecoveryCode(ctx context.Context, id, code string) error {
	out, err := s.db.ExecContext(ctx, s.sql(
		`DELETE FROM cronos_recovery_codes WHERE user_id = ? AND code_hash = ?`),
		id, identity.HashRecoveryCode(code))
	if err != nil {
		return err
	}
	if n, err := out.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return identity.ErrBadCode
	}
	return nil
}

/*
RemoveFactor turns protection off, and takes the recovery codes with it.

Codes left behind would be ten live credentials for an account with no second
factor — which is not a lesser risk than the factor was, it is a set of
passwords the owner believes are inert.
*/
func (s *Store) RemoveFactor(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	out, err := tx.ExecContext(ctx, s.sql(
		`DELETE FROM cronos_factors WHERE user_id = ?`), id)
	if err != nil {
		return err
	}
	if n, err := out.RowsAffected(); err == nil && n == 0 {
		return identity.ErrNoFactor
	}
	if _, err := tx.ExecContext(ctx, s.sql(
		`DELETE FROM cronos_recovery_codes WHERE user_id = ?`), id); err != nil {
		return err
	}
	return tx.Commit()
}
