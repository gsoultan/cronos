package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gsoultan/cronos/internal/app/schedule"
)

// Firing runs one schedule on demand.
//
// Narrower than the scheduler it borrows from: this is the only thing the API
// may ask of it, and handing the handler the whole service would let a future
// endpoint arm, disarm or re-time a schedule over HTTP by accident.
type Firing interface {
	Fire(ctx context.Context, name string) error
}

// Schedules serves what can be done to a schedule from outside.
//
// Running one now, and nothing else. Editing a schedule is publishing its
// definition, which the management API already does — a second way to change
// the same thing is a second thing to keep in step.
type Schedules struct {
	fires Firing
	auth  Principals
	log   *slog.Logger
}

// NewSchedules wires the handler.
func NewSchedules(f Firing, a Principals, log *slog.Logger) *Schedules {
	return &Schedules{fires: f, auth: a, log: log}
}

// ServeHTTP handles POST /v1/schedules/{name}/run.
func (h *Schedules) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	// The same bar as changing a definition. A run sends real documents to real
	// customers, so it is not something a reader may cause.
	if !pr.CanEdit() {
		fail(w, http.StatusForbidden, "You may not run schedules in this project.")
		return
	}
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
		return
	}

	name := r.PathValue("name")
	h.log.Info("running a schedule on request", "schedule", name, "by", pr.Subject)

	// Without cancel, keeping the request's values. A burst of five thousand
	// documents outlives the HTTP request that asked for it, and cancelling it
	// when the browser tab closes would leave a delivery that half happened —
	// which is worse to reconcile than one that did not start.
	if err := h.fires.Fire(context.WithoutCancel(r.Context()), name); err != nil {
		switch {
		case errors.Is(err, schedule.ErrNoSchedule):
			fail(w, http.StatusNotFound, "No such schedule.")
		case errors.Is(err, schedule.ErrRunning):
			fail(w, http.StatusConflict, "That schedule is already running.")
		default:
			fail(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// The record, not the result. What was delivered is in the run history,
	// which is one place rather than two that could disagree.
	send(w, http.StatusAccepted, map[string]string{"schedule": name, "status": "ran"})
}
