package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Managing who has access.

The store could create somebody and check their password, and nothing else.
That is enough to start a deployment and not enough to run one: when a person
leaves there was no way to revoke their access short of a SQL statement against
production, and the column that would have done it — disabled — existed in the
schema and was never written by anything.

Everything here is scoped to the acting principal's project. A user row carries
its own org and project, and every statement matches on both: an administrator
of one project reading or changing another's people is the failure this whole
model exists to prevent, and it is prevented in the WHERE clause rather than in
a check somebody can forget.
*/

// People lists who has access to this project, newest first.
func (s *Store) People(ctx context.Context, pr principal.Principal) ([]identity.User, error) {
	if err := tenant(pr); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT id, email, name, org, project, role, created_at, last_seen, disabled
		FROM cronos_users
		WHERE org = ? AND project = ?
		ORDER BY created_at DESC, email`), pr.OrgID, pr.ProjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []identity.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetRole changes what somebody may do.
func (s *Store) SetRole(ctx context.Context, pr principal.Principal, id, role string) error {
	if err := tenant(pr); err != nil {
		return err
	}
	return s.change(ctx, pr, id,
		`UPDATE cronos_users SET role = ? WHERE id = ? AND org = ? AND project = ?`, role)
}

/*
SetDisabled turns access off, or back on.

The row is kept. Deleting somebody removes the answer to "who ran this report
in March", which is a question an audit exists to answer and a departure does
not stop anybody asking. It also means re-enabling is a decision rather than a
re-creation, and that their run history still belongs to a person.
*/
func (s *Store) SetDisabled(ctx context.Context, pr principal.Principal, id string, disabled bool) error {
	if err := tenant(pr); err != nil {
		return err
	}
	return s.change(ctx, pr, id,
		`UPDATE cronos_users SET disabled = ? WHERE id = ? AND org = ? AND project = ?`, disabled)
}

// change applies one update and insists it matched something.
//
// Checked, so disabling somebody in another project is a not-found rather than
// a silent success that leaves an administrator believing they revoked access
// they did not.
func (s *Store) change(ctx context.Context, pr principal.Principal, id, query string, value any) error {
	result, err := s.db.ExecContext(ctx, s.sql(query), value, id, pr.OrgID, pr.ProjectID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: %s", identity.ErrNoUser, id)
	}
	return nil
}

/*
ChangePassword replaces somebody's own password, having checked the old one.

The current password is required and verified here rather than trusted from the
caller. A session is eight hours long and lives in a browser; without this,
anybody who borrowed one for a minute could lock the owner out of their own
account permanently, which is a worse outcome than the borrowed minute.
*/
func (s *Store) ChangePassword(ctx context.Context, id, current, next string) error {
	var hash string
	var disabled bool
	err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT password, disabled FROM cronos_users WHERE id = ?`), id).Scan(&hash, &disabled)
	if err != nil {
		return identity.ErrBadCredentials
	}
	if err := identity.Verify(hash, current); err != nil {
		return identity.ErrBadCredentials
	}
	if disabled {
		return identity.ErrBadCredentials
	}

	fresh, err := identity.Hash(next)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.sql(
		`UPDATE cronos_users SET password = ? WHERE id = ?`), fresh, id)
	return err
}

/*
Active reports whether a subject is an account here, and whether it may act.

Asked on every request that carries a portal token, because a token is signed
and lives eight hours: disabling somebody who holds one would otherwise do
nothing until it expired, and "revoked" that means "revoked by this evening" is
not what anybody means when they say it.

Two answers rather than one, and the first is the important one. Not every
portal token names an account: cronos-token mints them for pipelines and for a
portal built with one baked in, and their subject is whatever the operator
typed. Collapsing "no such account" into "not allowed" locked every one of
those out — which the live check found within a minute of it being written, and
which no unit test would have, because a unit test supplies a subject that
exists.

It is safe to serve a subject nobody has because this store never deletes a
person: leaving is disabling, and the row stays. So "unknown" means "not one of
our accounts" rather than "an account that used to be here".
*/
func (s *Store) Active(ctx context.Context, id string) (known, active bool, since time.Time) {
	var disabled bool
	var from sql.NullString

	// A left join, because most accounts have never had their sessions cut and
	// an inner one would report every one of them as unknown — which reads as
	// "machine credential" and would wave a disabled account straight through.
	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT u.disabled, c.at
		FROM cronos_users u
		LEFT JOIN cronos_sessions_cut c ON c.user_id = u.id
		WHERE u.id = ?`), id).
		Scan(&disabled, &from)
	if err != nil {
		return false, false, time.Time{}
	}
	if from.Valid {
		since = unstamp(from.String)
	}
	return true, !disabled, since
}

/*
EndSessions draws the line: every token minted before now stops working.

The one control that helps somebody whose laptop was taken. There is no list of
sessions to walk — a portal token is signed and carries no server-side record —
so this is a timestamp, and the standing check every request already makes is
where it takes effect.

Their own account only, which the caller enforces by passing the acting
subject. An administrator ending somebody else's sessions is `SetDisabled`, and
those are different things: one is "I lost my phone" and the other is "they no
longer work here".
*/
func (s *Store) EndSessions(ctx context.Context, id string) (time.Time, error) {
	/*
	   The next second, not this one.

	   A token's `iat` has second granularity, so a line drawn at 12:00:03.750
	   and a session minted at 12:00:03.100 are indistinguishable — and the
	   comparison has to spare same-second tokens, or the replacement this
	   endpoint mints is refused by the line it just drew. Driving it with two
	   real browsers found exactly that: a phone signed in 900ms before the
	   button was pressed survived it.

	   Rounding up removes the ambiguity instead of narrowing it. Everything
	   minted up to and including this second is on the far side; the
	   replacement is dated at the boundary and is the first token on this one.
	   The cost is that the replacement's `iat` is up to a second in the future,
	   which is well inside the 30 seconds of skew a token is already verified
	   with.
	*/
	line := s.now().Truncate(time.Second).Add(time.Second)

	// The account has to exist. Writing a cut for a subject that is not one
	// here would record a security event about nobody, and the caller wants to
	// be told rather than reassured.
	var exists int
	if err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT COUNT(*) FROM cronos_users WHERE id = ?`), id).Scan(&exists); err != nil {
		return time.Time{}, err
	}
	if exists == 0 {
		return time.Time{}, identity.ErrNoUser
	}

	if _, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_sessions_cut (user_id, at) VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET at = EXCLUDED.at`),
		id, stamp(line)); err != nil {
		return time.Time{}, err
	}
	return line, nil
}

func scanUser(row scanner) (identity.User, error) {
	var u identity.User
	var created string
	var seen sql.NullString

	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Org, &u.Project, &u.Role,
		&created, &seen, &u.Disabled); err != nil {
		return identity.User{}, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, created)
	u.LastSeen = optional(seen)
	return u, nil
}

/*
Upsert records somebody an identity provider vouched for.

Their account here, keyed by the provider's own subject rather than by their
email — an email is something people change, and a directory that keyed on one
would give somebody a new account and lose their history the day they married.

The role is applied on first sight and not on every sign-in. An administrator
who demoted somebody in Settings should not find the identity provider
promoting them back at the next login; a deployment that wants the directory to
own roles maps groups in the provider and does not change them here.
*/
func (s *Store) Upsert(ctx context.Context, u identity.User) (identity.User, error) {
	existing, err := s.User(ctx, u.ID)
	if err == nil {
		// Known. What the provider may still refresh is what it is
		// authoritative about: how they are addressed.
		if _, err := s.db.ExecContext(ctx, s.sql(
			`UPDATE cronos_users SET email = ?, name = ? WHERE id = ?`),
			normalise(u.Email), u.Name, u.ID); err != nil {
			return identity.User{}, err
		}
		if existing.Disabled {
			// Signed in with the directory and turned off here. The directory
			// is not the authority on whether somebody still works on this
			// project — that is what the People page is for.
			return identity.User{}, fmt.Errorf("%w: access is turned off", identity.ErrBadCredentials)
		}
		existing.Email, existing.Name = u.Email, u.Name
		return existing, nil
	}

	// New. A password nobody knows and nobody can use: this account signs in
	// through the provider, and leaving the column empty would make the
	// password path a way in.
	unusable, err := identity.Hash(identity.NewID() + identity.NewID())
	if err != nil {
		return identity.User{}, err
	}
	if _, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_users (id, email, name, password, org, project, role, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		u.ID, normalise(u.Email), u.Name, unusable, u.Org, u.Project, u.Role,
		stamp(s.now())); err != nil {
		return identity.User{}, err
	}
	return u, nil
}
