package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/app/schedule"
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
	auth    Principals
	log     *slog.Logger
	// projects resolves whose scheduler a resume belongs to. Absent for a
	// deployment with none, where resuming is a 404 like any other run.
	projects Projects
}

// NewRuns wires the handler.
func NewRuns(h History, a Principals, log *slog.Logger) *Runs {
	return &Runs{history: h, auth: a, log: log}
}

// WithProjects lets a run be resumed, which needs the scheduler that would
// re-send it.
func (h *Runs) WithProjects(p Projects) *Runs { h.projects = p; return h }

// ServeHTTP handles /v1/runs and /v1/runs/{id}.
func (h *Runs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	if strings.HasSuffix(r.URL.Path, "/resume") {
		h.resume(w, r, pr, r.PathValue("id"))
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

/*
Resuming re-sends a period's documents to whoever did not get one.

A port, because what it takes is a scheduler, a store and a burst runner — three
things this package deliberately does not know about. See boot.
*/
type Resuming interface {
	Resume(ctx context.Context, pr principal.Principal, runID string) error
}

/*
resume handles POST /v1/runs/{id}/resume.

The recovery for a burst that stopped halfway. Until this existed the only
option was to run the schedule again, which sends a second copy of the document
to everybody who already had one — worse, on an invoice, than the gap it was
fixing.
*/
func (h *Runs) resume(w http.ResponseWriter, r *http.Request, pr principal.Principal, id string) {
	// The same bar as running one. A resume sends real documents to real
	// customers; that it sends fewer of them does not make it a read.
	if !pr.CanEdit() {
		fail(w, http.StatusForbidden, "You may not run schedules in this project.")
		return
	}
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Use POST.")
		return
	}

	if h.projects == nil {
		fail(w, http.StatusNotFound, "No such run.")
		return
	}
	project, err := h.projects.Project(r.Context(), pr)
	if err != nil || project.Resumes == nil {
		// No scheduler armed here, or not this caller's project. On a replica
		// that is not the leader there is nothing to resume with, and saying
		// "no such run" is the same answer as for a run in another project.
		fail(w, http.StatusNotFound, "No such run.")
		return
	}

	h.log.Info("resuming a run on request", "run", id, "by", pr.Subject)
	audit(r.Context(), h.log, pr, ActionResume, id, Allowed, nil)

	// Without cancel, keeping the request's values — the same reason firing a
	// schedule does: a burst outlives the request that asked for it, and
	// cancelling on a closed tab leaves exactly the half-finished delivery this
	// endpoint exists to repair.
	if err := project.Resumes.Resume(context.WithoutCancel(r.Context()), pr, id); err != nil {
		switch {
		case errors.Is(err, schedule.ErrNoSchedule), errors.Is(err, publish.ErrNotFound):
			fail(w, http.StatusNotFound, "No such run.")
		case errors.Is(err, schedule.ErrRunning):
			fail(w, http.StatusConflict, "That schedule is already running.")
		default:
			fail(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// The record, not the result: what went where is in the run history, which
	// is one place rather than two that could disagree.
	send(w, http.StatusAccepted, map[string]string{"run": id, "status": "resumed"})
}
