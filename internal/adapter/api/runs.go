package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/history"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// History is where run records are read from.
//
// Declared here because this is where they are consumed, and both methods take
// the principal for the same reason the definition store does: the tenant is
// where to look, and one that must be passed cannot be forgotten.
type History interface {
	Runs(ctx context.Context, pr principal.Principal, limit int) ([]history.Run, error)
	Run(ctx context.Context, pr principal.Principal, id string) (history.Run, []history.Delivery, error)
}

// Runs serves the run history.
//
// Behind the admin key rather than the embed token. A run record names every
// recipient of a burst, so it is the one thing in cronos that must never be
// readable by an end customer holding a token for one of them.
type Runs struct {
	history History
	auth    *AdminKey
	log     *slog.Logger
}

// NewRuns wires the handler.
func NewRuns(h History, a *AdminKey, log *slog.Logger) *Runs {
	return &Runs{history: h, auth: a, log: log}
}

// ServeHTTP handles /v1/runs and /v1/runs/{id}.
func (h *Runs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	if r.Method != http.MethodGet {
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
		return
	}

	if id := r.PathValue("id"); id != "" {
		h.one(w, r, pr, id)
		return
	}
	h.list(w, r, pr)
}

func (h *Runs) list(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	runs, err := h.history.Runs(r.Context(), pr, limit)
	if err != nil {
		h.log.Error("listing runs failed", "err", err)
		fail(w, http.StatusInternalServerError, "Could not read the run history.")
		return
	}
	if runs == nil {
		runs = []history.Run{}
	}
	send(w, http.StatusOK, map[string]any{"runs": runs})
}

// one returns a run and everything it delivered.
//
// The deliveries are the answer to the question this endpoint exists for —
// which customer received what, at which address, after how many attempts.
func (h *Runs) one(w http.ResponseWriter, r *http.Request, pr principal.Principal, id string) {
	run, deliveries, err := h.history.Run(r.Context(), pr, id)
	if err != nil {
		if !errors.Is(err, publish.ErrNotFound) {
			h.log.Error("reading a run failed", "run", id, "err", err)
		}
		fail(w, http.StatusNotFound, "No such run.")
		return
	}
	if deliveries == nil {
		deliveries = []history.Delivery{}
	}
	send(w, http.StatusOK, map[string]any{"run": run, "deliveries": deliveries})
}
