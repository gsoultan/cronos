package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	app "github.com/gsoultan/cronos/internal/app/share"
	"github.com/gsoultan/cronos/internal/core/principal"
	core "github.com/gsoultan/cronos/internal/core/share"
)

// Sharing is what the API may ask of the share service.
type Sharing interface {
	Create(ctx context.Context, pr principal.Principal, req app.Request) (core.Share, error)
	List(ctx context.Context, pr principal.Principal) ([]core.Share, error)
	Revoke(ctx context.Context, pr principal.Principal, id string) error
	Open(ctx context.Context, id string) (string, core.Share, error)
}

// Shares serves links to a report for people who are not in the project.
type Shares struct {
	shares Sharing
	auth   Principals
	log    *slog.Logger
	now    func() time.Time
}

// NewShares wires the handler.
func NewShares(s Sharing, a Principals, log *slog.Logger) *Shares {
	return &Shares{shares: s, auth: a, log: log, now: time.Now}
}

// ServeHTTP handles /v1/shares, /v1/shares/{id} and /v1/shares/{id}/open.
func (h *Shares) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Opening is the one route with no principal, because the recipient has no
	// identity here — the link is what they have. Handled before authorising
	// anything, so that a missing header is not the answer to a valid link.
	if id != "" && r.URL.Path == "/v1/shares/"+id+"/open" {
		h.open(w, r, id)
		return
	}

	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}

	switch {
	case r.Method == http.MethodPost && id == "":
		h.create(w, r, pr)
	case r.Method == http.MethodGet && id == "":
		h.list(w, r, pr)
	case r.Method == http.MethodDelete && id != "":
		h.revoke(w, r, pr, id)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

// createRequest is what the portal sends.
type createRequest struct {
	Report string `json:"report"`
	// Days until it stops opening. Zero is never, which the interface offers.
	Days int `json:"days"`
}

func (h *Shares) create(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	var req createRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "That is not a share request.")
		return
	}

	sh, err := h.shares.Create(r.Context(), pr, app.Request{
		Report: req.Report,
		Days:   req.Days,
		// The sharer's own row constraint, never one they chose. Letting a
		// request name a scope would let an author mint a link to somebody
		// else's rows, which is the one thing row scope exists to prevent.
		Scope: pr.Scope,
	})
	if err != nil {
		h.refuse(w, err)
		return
	}
	h.log.Info("shared", "report", sh.Report, "share", sh.ID, "by", pr.Subject)
	send(w, http.StatusCreated, view(sh, h.now()))
}

func (h *Shares) list(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	shares, err := h.shares.List(r.Context(), pr)
	if err != nil {
		h.refuse(w, err)
		return
	}
	now := h.now()
	out := make([]shareView, 0, len(shares))
	for _, sh := range shares {
		out = append(out, view(sh, now))
	}
	send(w, http.StatusOK, map[string]any{"shares": out})
}

func (h *Shares) revoke(w http.ResponseWriter, r *http.Request, pr principal.Principal, id string) {
	if err := h.shares.Revoke(r.Context(), pr, id); err != nil {
		h.refuse(w, err)
		return
	}
	h.log.Info("share revoked", "share", id, "by", pr.Subject)
	w.WriteHeader(http.StatusNoContent)
}

// open exchanges a link for a token that reads the report behind it.
//
// The only unauthenticated route in the API, and the reason it can be: the id
// is the credential, it is checked against a record that can be withdrawn, and
// what it returns is an embed token for one report with the sharer's scope
// baked in. Nothing here is taken from the request.
func (h *Shares) open(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
		return
	}
	raw, sh, err := h.shares.Open(r.Context(), id)
	if err != nil {
		// One answer for never-existed, expired and revoked. Telling them
		// apart would let somebody holding a dead link learn that a live one
		// had been there.
		fail(w, http.StatusNotFound, "This link does not open. It may have expired, or been withdrawn.")
		return
	}
	// Not logged with the id of whoever opened it, because there is no such
	// person: a share is deliberately anonymous, and inventing an identity for
	// the log would be inventing one for the audit.
	h.log.Info("share opened", "share", sh.ID, "report", sh.Report)

	w.Header().Set("Cache-Control", "no-store")
	send(w, http.StatusOK, map[string]any{"token": raw, "report": sh.Report})
}

// shareView is what a listing shows. The token is not in it, and never is:
// it is minted per open and lives twenty-four hours at most.
type shareView struct {
	ID        string     `json:"id"`
	Report    string     `json:"report"`
	State     string     `json:"state"`
	CreatedBy string     `json:"createdBy"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	// Scoped says the link is narrowed to some of the rows rather than all of
	// them. The values are not returned: they are somebody's customer ids.
	Scoped bool `json:"scoped,omitempty"`
}

func view(sh core.Share, now time.Time) shareView {
	return shareView{
		ID: sh.ID, Report: sh.Report, State: sh.State(now),
		CreatedBy: sh.CreatedBy, CreatedAt: sh.CreatedAt,
		ExpiresAt: sh.ExpiresAt, RevokedAt: sh.RevokedAt,
		Scoped: len(sh.Scope) > 0,
	}
}

func (h *Shares) refuse(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrForbidden):
		fail(w, http.StatusForbidden, err.Error())
	case errors.Is(err, app.ErrScoped), errors.Is(err, app.ErrInvalid):
		fail(w, http.StatusBadRequest, err.Error())
	default:
		fail(w, http.StatusNotFound, "No such share.")
	}
}
