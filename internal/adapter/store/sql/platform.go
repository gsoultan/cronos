package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
		-- The SUM needs no COALESCE, unlike its neighbour in policy.go: a
		-- GROUP BY only produces a row where there is at least one to sum, so
		-- it is never NULL here.
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

/*
AddPerson creates an account in any organisation and project.

The verb this tier was missing. It could move somebody between organisations,
turn their access off and grant administration — and it could not create the
first account of a new customer, so onboarding meant adding the person to your
own project through the ordinary endpoint and then moving them. A two-step
workaround for the primary job of the tier.

Unscoped, like everything else here, which is exactly the thing no project
administrator may do: create an account somewhere they have no business.
*/
func (s *Store) AddPerson(ctx context.Context, u identity.User, password string) error {
	if u.Org == "" || u.Project == "" {
		return fmt.Errorf("%w: an account needs an organisation and a project",
			identity.ErrBadCredentials)
	}
	return s.CreateUser(ctx, u, password)
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

/*
ByEmail finds an account by the address it signs in with.

For the command line, which is handed an email rather than an id — nobody knows
their own `usr_9f2c4a…`, and a recovery path that requires looking one up in the
database first is a recovery path with a step people get wrong at the moment
they are least able to.

Unscoped, like everything else in this file: the account being rescued may be in
any project, and the caller is a person at a shell rather than a session.
*/
func (s *Store) ByEmail(ctx context.Context, email string) (identity.User, error) {
	row := s.db.QueryRowContext(ctx, s.sql(`
		SELECT id, email, name, org, project, role, created_at, last_seen, disabled
		FROM cronos_users WHERE email = ?`), normalise(email))

	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.User{}, identity.ErrNoUser
	}
	if err != nil {
		return identity.User{}, err
	}
	u.Platform = s.IsPlatformAdmin(ctx, u.ID)
	return u, nil
}

/*
FirstRun creates the deployment's first account, or refuses because there is one.

The whole of /setup's guarantee, moved into the database because that is the
only participant two cronos processes can agree on. The mutex it replaces was
enough for a double-clicked button and not enough for two processes brought up
against one empty database before anybody had been given the address — both
would have found it empty, and both would have made a deployment administrator.

One row with a fixed key, inserted in the same transaction as the account and
the grant. The second caller's insert violates the primary key and its whole
transaction rolls back, so there is no state in which two exist or in which an
account exists without the permission it was created for.
*/
func (s *Store) FirstRun(ctx context.Context, u identity.User, password string) error {
	hash, err := identity.Hash(password)
	if err != nil {
		return err
	}
	email := normalise(u.Email)
	if email == "" {
		return fmt.Errorf("%w: no email", identity.ErrBadCredentials)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := s.now()

	/*
	   First, to fail fast rather than for correctness.

	   The transaction is what makes a losing caller write nothing — the
	   rollback undoes the account whether it was inserted before this or after,
	   which a mutation test confirms by moving this line to the end and
	   changing no outcome. Putting it first only avoids doing the work of an
	   insert that is about to be thrown away.
	*/
	if _, err := tx.ExecContext(ctx, s.sql(
		`INSERT INTO cronos_setup (id, at, by_user) VALUES (1, ?, ?)`),
		stamp(now), email); err != nil {
		if !duplicate(err) {
			// A transient failure is not "somebody got here first". Reporting
			// it as one tells the person setting up a brand-new deployment
			// that it has already been set up, which is the one answer that
			// makes them stop trying.
			return err
		}
		return ErrAlreadySetUp
	}

	if _, err := tx.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_users (id, email, name, password, org, project, role, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		u.ID, email, u.Name, hash, u.Org, u.Project, u.Role, stamp(now)); err != nil {

		if duplicate(err) {
			return fmt.Errorf("%w: %s", identity.ErrExists, email)
		}
		return err
	}

	if _, err := tx.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_platform_admins (user_id, granted_at, granted_by)
		VALUES (?, ?, ?)`), u.ID, stamp(now), "setup"); err != nil {
		return err
	}
	return tx.Commit()
}

// ErrAlreadySetUp is a first run arriving second.
var ErrAlreadySetUp = errors.New("sql: this deployment has already been set up")

/*
SetUp reports whether a first run has happened.

From the marker row rather than by counting accounts. They agree today and would
not for ever: an account can be deleted, and a deployment whose last one was
removed is not a deployment nobody has ever configured — offering to set it up
again would be offering a stranger an administrator on a system that has been
running for a year.
*/
func (s *Store) SetUp(ctx context.Context) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, s.sql(
		`SELECT COUNT(*) FROM cronos_setup`)).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}

	/*
	   And a deployment that predates this table.

	   Migration 7 shipped after people were already running cronos, so an
	   existing deployment has accounts and no marker row. Counting them is what
	   tells those apart from a genuinely fresh install — without it, every
	   upgrade would offer its next visitor a deployment administrator.
	*/
	accounts, err := s.CountAccounts(ctx)
	return accounts > 0, err
}

/*
duplicate reports whether an error is a uniqueness violation.

By message, which is the ugly way and the portable one: the two drivers report
it differently — "UNIQUE constraint failed" and "duplicate key value violates
unique constraint" — and both contain the word. Matching on a pgx error code
would be exact and would leave SQLite needing its own branch, which is two
things to keep in step for a check with one meaning.

The distinction matters because the alternative is treating every failure as a
collision. A first run that could not reach the database would then be told the
deployment has already been set up, and the person would stop trying.
*/
func duplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate key")
}

/*
Rekey moves everything a placeholder tenancy holds to the name a first run gave.

A fresh install serves whatever CRONOS_ORG and CRONOS_PROJECT default to, because
nothing else exists yet, and it reconciles its definitions into the store under
that name before anybody has been asked what to call it. Then somebody opens
/setup, types "Acme", and lands in an empty portal — the definitions the
deployment shipped with are all filed under default/default, and every read is
scoped by the tenancy they are not in.

Found by running the development server the way somebody would after it started
connecting to its own API: set up, and the first page says "No reports in finance
yet" beside ten definitions the server had just loaded.

Called by the first run and by nothing else. Two guards make that safe: it moves
rows only from the tenancy the process booted with, and the caller has already
proved this deployment had never been set up.
*/
func (s *Store) Rekey(ctx context.Context, fromOrg, fromProject, toOrg, toProject string) error {
	if fromOrg == toOrg && fromProject == toProject {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	/*
	   Every table keyed by a tenant, named rather than discovered.

	   A loop over the schema would find them automatically and would also find
	   the next one somebody adds — which sounds like the point and is the
	   danger: a table added for a reason this function has never heard of would
	   be silently rewritten. Listing them means adding one is a decision.

	   cronos_users is deliberately absent. The only account that exists at this
	   moment is the one the first run just created, and it was created under
	   the new name already.
	*/
	for _, table := range []string{
		"cronos_definitions",
		"cronos_definition_versions",
		"cronos_runs",
		"cronos_shares",
		"cronos_invitations",
		"cronos_policies",
	} {
		if _, err := tx.ExecContext(ctx, s.sql(
			`UPDATE `+table+` SET org = ?, project = ? WHERE org = ? AND project = ?`),
			toOrg, toProject, fromOrg, fromProject); err != nil {
			return fmt.Errorf("rekeying %s: %w", table, err)
		}
	}
	return tx.Commit()
}

/*
Adopted records the tenancy a first run named, so a restart remembers it.

/setup adopted a name in memory and nowhere else. The store was re-keyed to the
new tenancy and the process came back believing it served the configured one, so
it found an empty store for its own tenant, adopted the definitions directory a
second time under the old name, and answered "you do not have access to this
project" to the administrator who had set it up ten seconds earlier.

Written in the same transaction as nothing else, deliberately: it is a fact
about the deployment rather than about the account, and a first run that
succeeded and then failed to record its name would leave a deployment that works
until its next restart — which is the failure being fixed, arrived at from the
other side.
*/
func (s *Store) Adopted(ctx context.Context, org, project string) error {
	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_tenancy (id, org, project, at) VALUES (1, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			org = excluded.org, project = excluded.project, at = excluded.at`),
		org, project, stamp(s.now().UTC()))
	return err
}

/*
Tenancy is the name a first run gave this deployment, if one did.

Empty strings where nothing has been adopted, which is every deployment that was
told what it serves in configuration — there is nothing to remember, and the
configured value is the answer.
*/
func (s *Store) Tenancy(ctx context.Context) (org, project string, err error) {
	row := s.db.QueryRowContext(ctx, s.sql(
		`SELECT org, project FROM cronos_tenancy WHERE id = 1`))
	switch err := row.Scan(&org, &project); {
	case errors.Is(err, sql.ErrNoRows):
		return "", "", nil
	case err != nil:
		return "", "", err
	}
	return org, project, nil
}
