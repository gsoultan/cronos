package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
)

// CreateUser registers somebody.
//
// The hash is made here rather than taken as an argument, so no caller has the
// opportunity to store a password by mistake.
func (s *Store) CreateUser(ctx context.Context, u identity.User, password string) error {
	hash, err := identity.Hash(password)
	if err != nil {
		return err
	}
	email := normalise(u.Email)
	if email == "" {
		return fmt.Errorf("%w: no email", identity.ErrBadCredentials)
	}

	_, err = s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_users (id, email, name, password, org, project, role, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		u.ID, email, u.Name, hash, u.Org, u.Project, u.Role, stamp(s.now()))

	if duplicate(err) {
		return fmt.Errorf("%w: %s", identity.ErrExists, email)
	}
	return err
}

// Authenticate checks an email and password.
//
// Every failure returns the same error. "No such user" told apart from "wrong
// password" is a way to enumerate who has an account, and knowing an address
// is registered is worth more to somebody phishing than it is to anybody
// honest.
func (s *Store) Authenticate(ctx context.Context, email, password string) (identity.User, error) {
	var u identity.User
	var hash string
	var created string
	var disabled bool

	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT id, email, name, password, org, project, role, created_at, disabled
		FROM cronos_users WHERE email = ?`), normalise(email)).
		Scan(&u.ID, &u.Email, &u.Name, &hash, &u.Org, &u.Project, &u.Role, &created, &disabled)

	if err != nil {
		// The work is done anyway, against a hash that cannot match. Returning
		// here would make an unknown address answer in a millisecond and a
		// known one in eighty, which is the whole enumeration attack.
		identity.Verify(decoy, password) //nolint:errcheck // deliberate
		return identity.User{}, identity.ErrBadCredentials
	}
	if err := identity.Verify(hash, password); err != nil {
		return identity.User{}, identity.ErrBadCredentials
	}
	if disabled {
		// Checked after the password, for the same reason: a disabled account
		// answering faster than a wrong password is a way to find one.
		return identity.User{}, identity.ErrBadCredentials
	}

	u.CreatedAt, _ = time.Parse(time.RFC3339, created)
	// Read here so the session can carry it, rather than asked on every
	// request. Taking it away cuts their sessions, so the claim never outlives
	// the grant by more than the moment between the two writes.
	u.Platform = s.IsPlatformAdmin(ctx, u.ID)
	return u, s.seen(ctx, u.ID)
}

// decoy is a real bcrypt hash of a value nobody knows.
//
// Its only job is to cost the same as a genuine comparison, so an unknown email
// takes as long to refuse as a known one.
const decoy = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// seen records the login. Failure to write it does not fail the login: being
// unable to note that somebody arrived is not a reason to turn them away.
func (s *Store) seen(ctx context.Context, id string) error {
	_, _ = s.db.ExecContext(ctx, s.sql(`UPDATE cronos_users SET last_seen = ? WHERE id = ?`),
		stamp(s.now()), id)
	return nil
}

// User returns somebody by id, without their hash.
func (s *Store) User(ctx context.Context, id string) (identity.User, error) {
	var u identity.User
	var created string
	var seen sql.NullString

	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT id, email, name, org, project, role, created_at, last_seen, disabled
		FROM cronos_users WHERE id = ?`), id).
		Scan(&u.ID, &u.Email, &u.Name, &u.Org, &u.Project, &u.Role, &created, &seen, &u.Disabled)
	if err != nil {
		return identity.User{}, fmt.Errorf("%w: %s", identity.ErrNotFound, id)
	}

	u.CreatedAt, _ = time.Parse(time.RFC3339, created)
	if seen.Valid && seen.String != "" {
		if t, err := time.Parse(time.RFC3339, seen.String); err == nil {
			u.LastSeen = &t
		}
	}
	return u, nil
}

// Users counts them, so a first-run check can tell an empty deployment from a
// configured one.
func (s *Store) Users(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM cronos_users`).Scan(&n)
	return n, err
}

// normalise makes an email comparable.
//
// Lowercased and trimmed, because somebody typing Dewi@Acme.example at six in
// the morning is the same person as dewi@acme.example — and a unique index that
// disagrees lets them create a second account rather than fail to log in.
func normalise(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
