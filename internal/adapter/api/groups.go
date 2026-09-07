package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gsoultan/cronos/internal/core/access"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Managing who may open what.

Two handlers, because they are two nouns an administrator thinks about
separately: groups of people, and grants on a report. Both are admin-only —
docs/tenancy.md already gives project admin "manage project members and
settings", and who may read which report is exactly that.

Nothing here is mounted without a store. A file-backed deployment has nowhere
to record a grant, and an endpoint that exists only to say so is one somebody
spends an afternoon probing.
*/

// Administering is what these handlers need from a store.
//
// One interface rather than two because one type satisfies it and splitting it
// would be a shape nothing has, but the methods are grouped so the seam a
// second implementation would fill is visible.
type Administering interface {
	Granting

	Groups(ctx context.Context, org, project string) ([]access.Group, error)
	CreateGroup(ctx context.Context, pr principal.Principal, name string, scope map[string]string) (access.Group, error)
	SetGroupScope(ctx context.Context, pr principal.Principal, id string, scope map[string]string) error
	DeleteGroup(ctx context.Context, pr principal.Principal, id string) error

	MembersOf(ctx context.Context, pr principal.Principal, id string) ([]string, error)
	AddToGroup(ctx context.Context, pr principal.Principal, id, userID string) error
	RemoveFromGroup(ctx context.Context, pr principal.Principal, id, userID string) error

	Grant(ctx context.Context, pr principal.Principal, g access.Grant) error
	RevokeGrant(ctx context.Context, pr principal.Principal, g access.Grant) error
	SetUserScope(ctx context.Context, pr principal.Principal, userID string, scope map[string]string) error
}

// Groups serves /v1/groups and everything under it.
type Groups struct {
	store Administering
	auth  Principals
	log   *slog.Logger
}

// NewGroups wires the handler.
func NewGroups(s Administering, a Principals, log *slog.Logger) *Groups {
	return &Groups{store: s, auth: a, log: log}
}

// admin resolves the caller and refuses anybody who may not administer here.
//
// One place, because every route in this file is the same answer to the same
// question and a handler that asks it per route is a handler that misses one.
func (h *Groups) admin(w http.ResponseWriter, r *http.Request) (principal.Principal, bool) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Sign in to manage access.")
		return pr, false
	}
	if !pr.CanAdminProject() {
		fail(w, http.StatusForbidden, "Only a project administrator may change who sees what.")
		return pr, false
	}
	return pr, true
}

func (h *Groups) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.admin(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")
	switch {
	case id == "" && r.Method == http.MethodGet:
		h.list(w, r, pr)
	case id == "" && r.Method == http.MethodPost:
		h.create(w, r, pr)
	case strings.HasSuffix(r.URL.Path, "/members"):
		h.members(w, r, pr, id)
	case r.Method == http.MethodPatch:
		h.setScope(w, r, pr, id)
	case r.Method == http.MethodDelete:
		h.delete(w, r, pr, id)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

func (h *Groups) list(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	groups, err := h.store.Groups(r.Context(), pr.OrgID, pr.ProjectID)
	if err != nil {
		h.log.Error("could not read groups", "err", err)
		fail(w, http.StatusInternalServerError, "Could not read the groups.")
		return
	}
	if groups == nil {
		// An empty list rather than null, so a browser can iterate it without
		// checking first.
		groups = []access.Group{}
	}
	send(w, http.StatusOK, map[string]any{"groups": groups})
}

type groupRequest struct {
	Name  string            `json:"name"`
	Scope map[string]string `json:"scope,omitempty"`
	// User is the account a membership call names.
	User string `json:"user,omitempty"`
}

func (h *Groups) create(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	req, ok := decodeGroup(w, r)
	if !ok {
		return
	}
	g, err := h.store.CreateGroup(r.Context(), pr, req.Name, req.Scope)
	if err != nil {
		h.log.Warn("could not create a group", "err", err, "name", req.Name)
		fail(w, http.StatusBadRequest, "Could not create that group. A name is required and must be unused.")
		return
	}
	audit(r.Context(), h.log, pr, ActionGroupCreate, g.Name, Allowed,
		map[string]any{"scope": req.Scope})
	send(w, http.StatusCreated, g)
}

func (h *Groups) setScope(w http.ResponseWriter, r *http.Request, pr principal.Principal, id string) {
	req, ok := decodeGroup(w, r)
	if !ok {
		return
	}
	if err := h.store.SetGroupScope(r.Context(), pr, id, req.Scope); err != nil {
		h.log.Error("could not set a group's scope", "err", err, "group", id)
		fail(w, http.StatusInternalServerError, "Could not change that group.")
		return
	}
	/*
	   Recorded, because this is the line that decides what a room full of
	   people can see. A scope change is not a settings change; it is every
	   member of that group reading different rows from their next request.
	*/
	audit(r.Context(), h.log, pr, ActionGroupScope, id, Allowed,
		map[string]any{"scope": req.Scope})
	send(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Groups) delete(w http.ResponseWriter, r *http.Request, pr principal.Principal, id string) {
	if err := h.store.DeleteGroup(r.Context(), pr, id); err != nil {
		h.log.Error("could not delete a group", "err", err, "group", id)
		fail(w, http.StatusInternalServerError, "Could not remove that group.")
		return
	}
	audit(r.Context(), h.log, pr, ActionGroupDelete, id, Allowed, nil)
	send(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Groups) members(w http.ResponseWriter, r *http.Request, pr principal.Principal, id string) {
	switch r.Method {
	case http.MethodGet:
		members, err := h.store.MembersOf(r.Context(), pr, id)
		if err != nil {
			fail(w, http.StatusNotFound, "No such group.")
			return
		}
		send(w, http.StatusOK, map[string]any{"members": members})

	case http.MethodPost, http.MethodDelete:
		req, ok := decodeGroup(w, r)
		if !ok {
			return
		}
		if req.User == "" {
			fail(w, http.StatusBadRequest, "Name the account to add or remove.")
			return
		}

		change, action := h.store.AddToGroup, ActionGroupJoin
		if r.Method == http.MethodDelete {
			change, action = h.store.RemoveFromGroup, ActionGroupLeave
		}
		if err := change(r.Context(), pr, id, req.User); err != nil {
			fail(w, http.StatusNotFound, "No such group.")
			return
		}
		audit(r.Context(), h.log, pr, action, id, Allowed,
			map[string]any{"account": req.User})
		send(w, http.StatusOK, map[string]any{"ok": true})

	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

func decodeGroup(w http.ResponseWriter, r *http.Request) (groupRequest, bool) {
	var req groupRequest
	if r.Body == nil || r.ContentLength == 0 {
		return req, true
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	// Unknown fields refused, the same as everywhere else: somebody sending
	// `scopes` rather than `scope` would otherwise create an unconfined group
	// and believe it was confined.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "That is not a group.")
		return req, false
	}
	return req, true
}

/* -- grants on one report -------------------------------------------------- */

// ReportGrants serves /v1/reports/{name}/grants.
//
// Under the report rather than under a grants collection, because that is the
// question an administrator arrives with: not "what grants exist" but "who can
// see this".
type ReportGrants struct {
	store Administering
	auth  Principals
	log   *slog.Logger
}

// NewReportGrants wires the handler.
func NewReportGrants(s Administering, a Principals, log *slog.Logger) *ReportGrants {
	return &ReportGrants{store: s, auth: a, log: log}
}

type grantRequest struct {
	Kind    access.Kind `json:"kind"`
	Subject string      `json:"subject"`
}

func (h *ReportGrants) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Sign in to manage access.")
		return
	}
	if !pr.CanAdminProject() {
		fail(w, http.StatusForbidden, "Only a project administrator may change who sees what.")
		return
	}

	report := r.PathValue("name")
	if report == "" {
		fail(w, http.StatusBadRequest, "Name the report.")
		return
	}

	switch r.Method {
	case http.MethodGet:
		all, err := h.store.Grants(r.Context(), pr.OrgID, pr.ProjectID)
		if err != nil {
			h.log.Error("could not read grants", "err", err)
			fail(w, http.StatusInternalServerError, "Could not read who may open this.")
			return
		}
		mine := access.For(report, all)
		send(w, http.StatusOK, map[string]any{
			"grants": mine,
			// Said explicitly rather than left to be inferred from an empty
			// list, because "nobody has restricted this" and "this is
			// restricted to nobody" are opposite states that both render as
			// zero rows.
			"restricted": access.Restricted(mine),
		})

	case http.MethodPost, http.MethodDelete:
		var req grantRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			fail(w, http.StatusBadRequest, "That is not a grant.")
			return
		}

		g := access.Grant{Report: report, Kind: req.Kind, Subject: req.Subject}
		change, action := h.store.Grant, ActionGrant
		if r.Method == http.MethodDelete {
			change, action = h.store.RevokeGrant, ActionRevokeGrant
		}
		if err := change(r.Context(), pr, g); err != nil {
			h.log.Warn("could not change a grant", "err", err, "report", report)
			fail(w, http.StatusBadRequest,
				"A grant names a report and either a user or a group.")
			return
		}
		/*
		   The first grant on a report is the moment it stops being visible to
		   the project, so it is recorded with what it was and who did it. An
		   audit that cannot say who narrowed a report answers half the
		   question an auditor asks.
		*/
		audit(r.Context(), h.log, pr, action, report, Allowed,
			map[string]any{"kind": string(req.Kind), "subject": req.Subject})
		send(w, http.StatusOK, map[string]any{"ok": true})

	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}
