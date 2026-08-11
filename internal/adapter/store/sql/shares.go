package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/share"
)

// Shared records a link somebody handed out.
func (s *Store) Shared(ctx context.Context, pr principal.Principal, sh share.Share) error {
	if err := tenant(pr); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_shares
			(id, org, project, report, scope, created_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		sh.ID, pr.OrgID, pr.ProjectID, sh.Report, encodeScope(sh.Scope),
		sh.CreatedBy, stamp(sh.CreatedAt), stampp(sh.ExpiresAt))
	return err
}

// Shares lists what this project has handed out, newest first.
func (s *Store) Shares(ctx context.Context, pr principal.Principal) ([]share.Share, error) {
	if err := tenant(pr); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT id, org, project, report, scope, created_by, created_at, expires_at, revoked_at
		FROM cronos_shares
		WHERE org = ? AND project = ?
		ORDER BY created_at DESC, id`), pr.OrgID, pr.ProjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []share.Share
	for rows.Next() {
		sh, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// Share returns one, by the id a token carries.
//
// No principal: this is the lookup a request makes while proving who it is,
// and the tenant it belongs to is what the caller then checks the token
// against. Taking a principal here would mean trusting the token's own claim
// about which project it is in to find the record that would have said.
func (s *Store) Share(ctx context.Context, id string) (share.Share, error) {
	row := s.db.QueryRowContext(ctx, s.sql(`
		SELECT id, org, project, report, scope, created_by, created_at, expires_at, revoked_at
		FROM cronos_shares WHERE id = ?`), id)

	sh, err := scanShare(row)
	if err != nil {
		return share.Share{}, fmt.Errorf("%w: share %q", publish.ErrNotFound, id)
	}
	return sh, nil
}

// Revoke withdraws a share.
//
// The row is kept. "This link was revoked on the 3rd" is the answer somebody
// needs when a customer says the link stopped working, and a deleted row
// answers it with silence.
func (s *Store) Revoke(ctx context.Context, pr principal.Principal, id string, at time.Time) error {
	if err := tenant(pr); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, s.sql(`
		UPDATE cronos_shares SET revoked_at = ?
		WHERE id = ? AND org = ? AND project = ? AND revoked_at IS NULL`),
		stamp(at), id, pr.OrgID, pr.ProjectID)
	if err != nil {
		return err
	}
	// Checked, so revoking something already revoked — or belonging to another
	// project — is not a silent success that leaves somebody believing a live
	// link is dead.
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: share %q", publish.ErrNotFound, id)
	}
	return nil
}

func scanShare(row scanner) (share.Share, error) {
	var sh share.Share
	var scope, created string
	var expires, revoked sql.NullString

	err := row.Scan(&sh.ID, &sh.Org, &sh.Project, &sh.Report, &scope,
		&sh.CreatedBy, &created, &expires, &revoked)
	if err != nil {
		return share.Share{}, err
	}
	sh.Scope = decodeScope(scope)
	sh.CreatedAt, _ = time.Parse(time.RFC3339, created)
	sh.ExpiresAt = optional(expires)
	sh.RevokedAt = optional(revoked)
	return sh, nil
}

func optional(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v.String)
	if err != nil {
		return nil
	}
	return &t
}

// encodeScope stores the row constraint as JSON.
//
// One column rather than a child table: a scope is read whole, written once
// and never queried across, and the join would exist to serve no question
// anybody asks.
func encodeScope(scope map[string]string) string {
	if len(scope) == 0 {
		return ""
	}
	b, err := json.Marshal(scope)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeScope(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	var scope map[string]string
	if err := json.Unmarshal([]byte(raw), &scope); err != nil {
		return nil
	}
	return scope
}
