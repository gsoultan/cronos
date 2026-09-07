package sql

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gsoultan/cronos/internal/core/access"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Groups, report grants, and the row scope a person reads through.

Three reads on the sign-in path and one on every catalogue, so the shapes here
are chosen to be one query each rather than one per row. A permission check that
costs a round trip per report is a permission check somebody caches badly.

Everything is scoped by org and project in the statement itself, never by the
caller remembering to. A grant is a permission, and a permission query that can
be written without its tenancy is one that eventually is.
*/

// Group is a named set of people and the rows they read through.
type Group struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Scope   map[string]string `json:"scope,omitempty"`
	Members int               `json:"members"`
}

// ErrScopeConflict means two groups confine the same field differently, so
// there is no single answer to what their shared member may read.
var ErrScopeConflict = errors.New("store: two groups disagree about the same scope field")

/*
Confinement is what a person reads through, resolved once at sign-in.

Their own scope wins outright where they have one. Otherwise their groups' are
merged, and two groups setting the same field to different values is refused
rather than resolved: picking one silently is how somebody ends up reading a
region nobody granted them, and picking the union is how a confinement widens
by adding a group that was supposed to narrow.
*/
func (s *Store) Confinement(ctx context.Context, userID string) (map[string]string, error) {
	own, err := s.userScope(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(own) > 0 {
		return own, nil
	}

	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT g.name, g.scope FROM cronos_groups g
		JOIN cronos_group_members m ON m.group_id = g.id
		WHERE m.user_id = ?`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	merged := map[string]string{}
	from := map[string]string{} // field -> the group that set it, for the message
	for rows.Next() {
		var name, raw string
		if err := rows.Scan(&name, &raw); err != nil {
			return nil, err
		}
		scope, err := confinementFrom(raw)
		if err != nil {
			return nil, fmt.Errorf("group %q: %w", name, err)
		}
		for field, value := range scope {
			if was, seen := merged[field]; seen && was != value {
				return nil, fmt.Errorf("%w: %q says %s=%q and %q says %s=%q",
					ErrScopeConflict, from[field], field, was, name, field, value)
			}
			merged[field], from[field] = value, name
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(merged) == 0 {
		// Nil rather than an empty map, because principal.RowScoped counts
		// entries and an empty map must not read as a confinement.
		return nil, nil
	}
	return merged, nil
}

func (s *Store) userScope(ctx context.Context, userID string) (map[string]string, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT scope FROM cronos_user_scopes WHERE user_id = ?`), userID).Scan(&raw)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return confinementFrom(raw)
}

// GroupsOf names the groups somebody belongs to, for the access decision.
func (s *Store) GroupsOf(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT g.name FROM cronos_groups g
		JOIN cronos_group_members m ON m.group_id = g.id
		WHERE m.user_id = ?
		ORDER BY g.name`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

/*
Grants reads every grant in a project, in one query.

The whole project rather than one report because the catalogue needs all of
them at once, and asking per report is a query per row of a list. Callers
narrow with access.For, which is the only place that filtering is written.
*/
func (s *Store) Grants(ctx context.Context, org, project string) ([]access.Grant, error) {
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT report, kind, subject FROM cronos_report_grants
		WHERE org = ? AND project = ?`), org, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []access.Grant
	for rows.Next() {
		var g access.Grant
		if err := rows.Scan(&g.Report, &g.Kind, &g.Subject); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Grant records that somebody may open a report. Idempotent: granting twice is
// what a second administrator does, and it is not an error.
func (s *Store) Grant(ctx context.Context, pr principal.Principal, g access.Grant) error {
	if g.Report == "" || g.Subject == "" {
		// A blank subject matches nobody in access.Allowed, so a row holding
		// one is a permission that looks granted and is not.
		return fmt.Errorf("a grant names a report and a subject")
	}
	if g.Kind != access.User && g.Kind != access.Group {
		return fmt.Errorf("a grant is to a %q or a %q, not %q", access.User, access.Group, g.Kind)
	}

	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_report_grants (org, project, report, kind, subject, granted_at, granted_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (org, project, report, kind, subject) DO NOTHING`),
		pr.OrgID, pr.ProjectID, g.Report, string(g.Kind), g.Subject,
		time.Now().UTC().Format(time.RFC3339), pr.Subject)
	return err
}

// RevokeGrant takes one back. Scoped to the caller's project in the statement, so a
// grant in another tenant cannot be removed by naming it.
func (s *Store) RevokeGrant(ctx context.Context, pr principal.Principal, g access.Grant) error {
	_, err := s.db.ExecContext(ctx, s.sql(`
		DELETE FROM cronos_report_grants
		WHERE org = ? AND project = ? AND report = ? AND kind = ? AND subject = ?`),
		pr.OrgID, pr.ProjectID, g.Report, string(g.Kind), g.Subject)
	return err
}

/*
confinementFrom reads a stored scope, treating empty as none.

Distinct from decodeScope in shares.go, which returns nil for malformed JSON,
and the difference is the direction each fails in. A share with no scope matches
no rows, so swallowing the error there fails closed. A person with no
confinement reads everything, so swallowing it here would quietly lift a
confinement somebody set — which is why this one returns the error and its
callers refuse the sign-in rather than widening it.
*/
func confinementFrom(raw string) (map[string]string, error) {
	if raw == "" || raw == "{}" {
		return nil, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("scope is not a JSON object of strings: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

/* -- managing groups ------------------------------------------------------- */

// Groups lists a project's groups with their membership counts.
func (s *Store) Groups(ctx context.Context, org, project string) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT g.id, g.name, g.scope,
		       (SELECT COUNT(*) FROM cronos_group_members m WHERE m.group_id = g.id)
		FROM cronos_groups g
		WHERE g.org = ? AND g.project = ?
		ORDER BY g.name`), org, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		var raw string
		if err := rows.Scan(&g.ID, &g.Name, &raw, &g.Members); err != nil {
			return nil, err
		}
		if g.Scope, err = confinementFrom(raw); err != nil {
			return nil, fmt.Errorf("group %q: %w", g.Name, err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// CreateGroup adds one, with the scope its members will read through.
func (s *Store) CreateGroup(ctx context.Context, pr principal.Principal, name string,
	scope map[string]string) (Group, error) {

	if name == "" {
		return Group{}, fmt.Errorf("a group has a name")
	}
	raw, err := json.Marshal(scopeOrEmpty(scope))
	if err != nil {
		return Group{}, err
	}

	g := Group{ID: groupID(), Name: name, Scope: scope}
	_, err = s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_groups (id, org, project, name, scope, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`),
		g.ID, pr.OrgID, pr.ProjectID, name, string(raw),
		time.Now().UTC().Format(time.RFC3339))
	return g, err
}

// SetGroupScope changes what a group's members read through.
//
// Scoped to the caller's project in the statement: a group id from another
// tenant names nothing here, so it cannot be re-pointed by guessing one.
func (s *Store) SetGroupScope(ctx context.Context, pr principal.Principal,
	id string, scope map[string]string) error {

	raw, err := json.Marshal(scopeOrEmpty(scope))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.sql(`
		UPDATE cronos_groups SET scope = ?
		WHERE id = ? AND org = ? AND project = ?`),
		string(raw), id, pr.OrgID, pr.ProjectID)
	return err
}

/*
DeleteGroup removes a group, its membership and every grant naming it.

All three, because a grant to a group nobody can join is a permission that
looks live in a list and opens nothing — and worse, a group recreated under the
same name would silently inherit it.
*/
func (s *Store) DeleteGroup(ctx context.Context, pr principal.Principal, id string) error {
	var name string
	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT name FROM cronos_groups WHERE id = ? AND org = ? AND project = ?`),
		id, pr.OrgID, pr.ProjectID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM cronos_group_members WHERE group_id = ?`, []any{id}},
		{`DELETE FROM cronos_report_grants
		  WHERE org = ? AND project = ? AND kind = 'group' AND subject = ?`,
			[]any{pr.OrgID, pr.ProjectID, name}},
		{`DELETE FROM cronos_groups WHERE id = ? AND org = ? AND project = ?`,
			[]any{id, pr.OrgID, pr.ProjectID}},
	} {
		if _, err := s.db.ExecContext(ctx, s.sql(statement.sql), statement.args...); err != nil {
			return err
		}
	}
	return nil
}

// AddToGroup puts somebody in one. Idempotent, like Grant.
func (s *Store) AddToGroup(ctx context.Context, pr principal.Principal, id, userID string) error {
	if !s.ownsGroup(ctx, pr, id) {
		return fmt.Errorf("no such group")
	}
	_, err := s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_group_members (group_id, user_id, added_at) VALUES (?, ?, ?)
		ON CONFLICT (group_id, user_id) DO NOTHING`),
		id, userID, time.Now().UTC().Format(time.RFC3339))
	return err
}

// RemoveFromGroup takes somebody out.
func (s *Store) RemoveFromGroup(ctx context.Context, pr principal.Principal, id, userID string) error {
	if !s.ownsGroup(ctx, pr, id) {
		return fmt.Errorf("no such group")
	}
	_, err := s.db.ExecContext(ctx, s.sql(`
		DELETE FROM cronos_group_members WHERE group_id = ? AND user_id = ?`), id, userID)
	return err
}

// MembersOf names the accounts in a group.
func (s *Store) MembersOf(ctx context.Context, pr principal.Principal, id string) ([]string, error) {
	if !s.ownsGroup(ctx, pr, id) {
		return nil, fmt.Errorf("no such group")
	}
	rows, err := s.db.QueryContext(ctx, s.sql(`
		SELECT user_id FROM cronos_group_members WHERE group_id = ? ORDER BY user_id`), id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetUserScope confines one person directly, overriding their groups'.
func (s *Store) SetUserScope(ctx context.Context, pr principal.Principal,
	userID string, scope map[string]string) error {

	if len(scope) == 0 {
		_, err := s.db.ExecContext(ctx, s.sql(`
			DELETE FROM cronos_user_scopes WHERE user_id = ?`), userID)
		return err
	}
	raw, err := json.Marshal(scope)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, s.sql(`
		INSERT INTO cronos_user_scopes (user_id, scope, set_at, set_by) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET scope = ?, set_at = ?, set_by = ?`),
		userID, string(raw), now, pr.Subject, string(raw), now, pr.Subject)
	return err
}

// ownsGroup reports whether the group is the caller's project's, so every
// membership write is refused for a group in another tenant.
func (s *Store) ownsGroup(ctx context.Context, pr principal.Principal, id string) bool {
	var one int
	err := s.db.QueryRowContext(ctx, s.sql(`
		SELECT 1 FROM cronos_groups WHERE id = ? AND org = ? AND project = ?`),
		id, pr.OrgID, pr.ProjectID).Scan(&one)
	return err == nil
}

// scopeOrEmpty keeps a nil scope out of the column as "null".
func scopeOrEmpty(s map[string]string) map[string]string {
	if s == nil {
		return map[string]string{}
	}
	return s
}

// groupID names a group.
//
// Random rather than derived from the name: a group can be renamed, and an id
// that spells its old name is a grant nobody can read the meaning of.
func groupID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice, and a group that could not be
		// named must not be one that silently shares an id with another.
		return "grp_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "grp_" + hex.EncodeToString(b[:])
}
