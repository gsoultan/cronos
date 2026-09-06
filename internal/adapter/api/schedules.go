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
	// projects resolves whose scheduler this is. Firing another project's
	// schedule sends real documents to their customers.
	projects Projects
	auth     Principals
	log      *slog.Logger
}

// NewSchedules wires the handler.
func NewSchedules(projects Projects, a Principals, log *slog.Logger) *Schedules {
	return &Schedules{projects: projects, auth: a, log: log}
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

	project, err := h.projects.Project(r.Context(), pr)
	if err != nil {
		// Not this caller's project. Refusing to confirm the name is the point:
		// somebody probing another tenant's deployment learns nothing from a
		// 404 and learns the schedule exists from anything else.
		fail(w, http.StatusNotFound, "No such schedule.")
		return
	}
	if project.Fires == nil {
		/*
		   Resolved, and this instance is not the one that can run it.

		   Told apart from the 404 above because the two have nothing in common
		   but the outcome. This caller is in the project and the catalogue has
		   already shown them the schedule, so denying its existence is a lie
		   that costs somebody the afternoon they spend checking the spelling —
		   which is right. Nothing is disclosed by saying so: they could read
		   the same fact off cronos_scheduler_armed.

		   503 rather than 404 because another replica may well be armed, and a
		   status that means "not here, try again" is the one a load balancer
		   reads correctly.
		*/
		fail(w, http.StatusServiceUnavailable,
			"No scheduler is armed on this instance. Set CRONOS_SCHEDULER=1 here, "+
				"or send this to a replica that has it.")
		return
	}

	name := r.PathValue("name")
	h.log.Info("running a schedule on request", "schedule", name, "by", pr.Subject)

	// Without cancel, keeping the request's values. A burst of five thousand
	// documents outlives the HTTP request that asked for it, and cancelling it
	// when the browser tab closes would leave a delivery that half happened —
	// which is worse to reconcile than one that did not start.
	if err := project.Fires.Fire(context.WithoutCancel(r.Context()), name); err != nil {
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
