package sql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gsoultan/cronos/internal/app/vault"
)

/*
A project's stored secrets, sealed.

This file never sees a value. It is handed ciphertext by vault and hands
ciphertext back, so there is no query here whose result could be a password —
which is what makes it safe for this table to be in every backup.

Times to the nanosecond rather than the second every other table uses: Watch
tells a changed secret from an unchanged one by when it was set, and two changes
inside one second would otherwise look like none.
*/

// Sealed is one secret's ciphertext, and whether there is one.
func (s *Store) Sealed(ctx context.Context, org, project, name string) ([]byte, bool, error) {
	var sealed []byte
	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT sealed FROM cronos_secrets WHERE org = ? AND project = ? AND name = ?`),
		org, project, name).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return sealed, true, nil
}

// SecretsOf lists one project's secrets: names and who set them, no values.
func (s *Store) SecretsOf(ctx context.Context, org, project string) ([]vault.Stored, error) {
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT name, updated_at, updated_by FROM cronos_secrets
		WHERE org = ? AND project = ? ORDER BY name`), org, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []vault.Stored
	for rows.Next() {
		st := vault.Stored{Org: org, Project: project}
		var at string
		if err := rows.Scan(&st.Name, &at, &st.UpdatedBy); err != nil {
			return nil, err
		}
		st.UpdatedAt = unstampNano(at)
		out = append(out, st)
	}
	return out, rows.Err()
}

// PutSecret stores a sealed value, replacing the one under the same name.
func (s *Store) PutSecret(ctx context.Context, st vault.Stored) error {
	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_secrets (org, project, name, sealed, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (org, project, name) DO UPDATE SET
		  sealed = EXCLUDED.sealed,
		  updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by`),
		st.Org, st.Project, st.Name, st.Sealed, stampNano(st.UpdatedAt), st.UpdatedBy)
	return err
}

// DeleteSecret removes one, reporting whether there was one to remove.
func (s *Store) DeleteSecret(ctx context.Context, org, project, name string) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.sql(`
		DELETE FROM cronos_secrets WHERE org = ? AND project = ? AND name = ?`),
		org, project, name)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// AllSecrets is every sealed value in the deployment, for resealing under a new
// key at startup. The one read here that crosses tenants, and it returns
// nothing anybody can open without the key.
func (s *Store) AllSecrets(ctx context.Context) ([]vault.Stored, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT org, project, name, sealed, updated_at, updated_by FROM cronos_secrets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []vault.Stored
	for rows.Next() {
		var st vault.Stored
		var at string
		if err := rows.Scan(&st.Org, &st.Project, &st.Name, &st.Sealed, &at, &st.UpdatedBy); err != nil {
			return nil, err
		}
		st.UpdatedAt = unstampNano(at)
		out = append(out, st)
	}
	return out, rows.Err()
}

// Reseal replaces a value sealed under a retired key, only if it is still the
// value that was read. The time and author stay: sealing again changes nothing
// anybody set.
func (s *Store) Reseal(ctx context.Context, st vault.Stored, was []byte) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.sql(`
		UPDATE cronos_secrets SET sealed = ?
		WHERE org = ? AND project = ? AND name = ? AND sealed = ?`),
		st.Sealed, st.Org, st.Project, st.Name, was)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func stampNano(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func unstampNano(s string) time.Time {
	at, _ := time.Parse(time.RFC3339Nano, s)
	return at
}
