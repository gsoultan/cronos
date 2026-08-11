package sql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Store keeps definitions in a database.
type Store struct {
	db *sql.DB
	// driver decides the DDL. The statements are portable; the types they
	// declare are not.
	driver string
	// mark writes the placeholder the driver expects. Postgres numbers them
	// and SQLite does not, and this is the only difference between the two
	// that reaches these statements.
	mark func(n int) string
	now  func() time.Time
}

// New returns a Store over db, using placeholders of the given style.
//
// Take the marker rather than sniff the driver: a caller already had to decide
// which database this is in order to open it, and guessing here would be a
// second place for that decision to be made differently.
func New(db *sql.DB, mark func(n int) string) *Store {
	return &Store{db: db, mark: mark, now: time.Now, driver: "sqlite"}
}

// ForDriver names the database, so Migrate declares types it has.
func (s *Store) ForDriver(driver string) *Store {
	s.driver = driver
	return s
}

// Dollar and Question are the two placeholder styles.
func Dollar(n int) string { return fmt.Sprintf("$%d", n) }
func Question(int) string { return "?" }

// WithClock makes the stored timestamps predictable in a test.
func (s *Store) WithClock(now func() time.Time) *Store { s.now = now; return s }

// Migrate creates the tables if they are absent.
//
// Statement by statement. Postgres refuses multiple commands in one prepared
// exec, and the driver prepares anything with parameters — so a schema that
// arrives as one string works on SQLite and fails on Postgres, which is the
// same asymmetry the types had.
func (s *Store) Migrate(ctx context.Context) error {
	for _, stmt := range strings.Split(Schema(s.driver), ";") {
		if strings.TrimSpace(stripComments(stmt)) == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sql: migrating: %w", err)
		}
	}
	return nil
}

// stripComments removes the SQL comments, so a statement that is only a
// comment is not sent as an empty query.
func stripComments(stmt string) string {
	var kept []string
	for _, line := range strings.Split(stmt, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// Version is the content address of a document.
func Version(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])[:12]
}

// Put stores a definition and keeps the previous content addressable.
func (s *Store) Put(ctx context.Context, pr principal.Principal,
	kind, name string, raw []byte) (string, error) {

	if err := tenant(pr); err != nil {
		return "", err
	}
	version := Version(raw)
	at := s.now().UTC().Format(time.RFC3339)

	// One transaction, because a definition whose history is missing cannot be
	// reproduced and a history entry nothing points at is merely inert. If
	// only one can exist, it must be the history.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback() //nolint:errcheck // committed below; this is the failure path

	if _, err := tx.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_definition_versions
			(org, project, kind, name, version, body, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (org, project, kind, name, version) DO NOTHING`),
		pr.OrgID, pr.ProjectID, kind, name, version, raw, at, pr.Subject); err != nil {
		return "", fmt.Errorf("sql: recording version: %w", err)
	}

	if _, err := tx.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_definitions
			(org, project, kind, name, version, body, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (org, project, kind, name) DO UPDATE SET
			version = excluded.version, body = excluded.body,
			updated_at = excluded.updated_at, updated_by = excluded.updated_by`),
		pr.OrgID, pr.ProjectID, kind, name, version, raw, at, pr.Subject); err != nil {
		return "", fmt.Errorf("sql: storing definition: %w", err)
	}

	return version, tx.Commit()
}

// Get returns the stored document.
func (s *Store) Get(ctx context.Context, pr principal.Principal, kind, name string) ([]byte, error) {
	if err := tenant(pr); err != nil {
		return nil, err
	}
	var body []byte
	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT body FROM cronos_definitions
		WHERE org = ? AND project = ? AND kind = ? AND name = ?`),
		pr.OrgID, pr.ProjectID, kind, name).Scan(&body)

	if err != nil {
		// Not found and belongs-to-another-tenant are the same answer. Telling
		// them apart would let a caller enumerate what other projects have.
		return nil, fmt.Errorf("%w: %s %q", publish.ErrNotFound, kind, name)
	}
	return body, nil
}

// Delete removes a definition. Its versions are kept: a run that used one must
// still be reproducible, and deleting a definition is not a claim that it
// never existed.
func (s *Store) Delete(ctx context.Context, pr principal.Principal, kind, name string) error {
	if err := tenant(pr); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, s.sql(`
		DELETE FROM cronos_definitions
		WHERE org = ? AND project = ? AND kind = ? AND name = ?`),
		pr.OrgID, pr.ProjectID, kind, name)
	if err != nil {
		return err
	}
	// Checked, so deleting something that was never there is a 404 rather than
	// a silent success that leaves a pipeline believing it cleaned up.
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: %s %q", publish.ErrNotFound, kind, name)
	}
	return nil
}

// List returns everything this tenant has.
func (s *Store) List(ctx context.Context, pr principal.Principal) ([]publish.Entry, error) {
	if err := tenant(pr); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT kind, name, version FROM cronos_definitions
		WHERE org = ? AND project = ?
		ORDER BY kind, name`), pr.OrgID, pr.ProjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []publish.Entry
	for rows.Next() {
		var e publish.Entry
		if err := rows.Scan(&e.Kind, &e.Name, &e.Version); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Versions lists a definition's history, newest first.
func (s *Store) Versions(ctx context.Context, pr principal.Principal, kind, name string) ([]string, error) {
	if err := tenant(pr); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT version FROM cronos_definition_versions
		WHERE org = ? AND project = ? AND kind = ? AND name = ?
		ORDER BY created_at DESC, version`),
		pr.OrgID, pr.ProjectID, kind, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// AtVersion returns the exact bytes a run recorded.
func (s *Store) AtVersion(ctx context.Context, pr principal.Principal,
	kind, name, version string) ([]byte, error) {

	if err := tenant(pr); err != nil {
		return nil, err
	}
	var body []byte
	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT body FROM cronos_definition_versions
		WHERE org = ? AND project = ? AND kind = ? AND name = ? AND version = ?`),
		pr.OrgID, pr.ProjectID, kind, name, version).Scan(&body)
	if err != nil {
		return nil, fmt.Errorf("%w: %s %q at %s", publish.ErrNotFound, kind, name, version)
	}
	return body, nil
}

// tenant refuses a principal with nowhere to act.
//
// An empty organization or project would match rows written with empty ones,
// which is a tenant nobody meant to create and everybody can reach.
func tenant(pr principal.Principal) error {
	if pr.OrgID == "" || pr.ProjectID == "" {
		return fmt.Errorf("%w: no organization or project", publish.ErrForbidden)
	}
	return nil
}
