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
	// gate refuses to run a schedule whose report the caller may not open.
	// Running one renders and delivers that report on demand, which is the
	// same act as sending it.
	gate gate
}

// NewSchedules wires the handler.
func NewSchedules(projects Projects, a Principals, log *slog.Logger) *Schedules {
	return &Schedules{projects: projects, auth: a, log: log}
}

// WithGrants refuses to run a schedule for a report the caller has not been
// granted.
func (h *Schedules) WithGrants(g Granting) *Schedules {
	h.gate = gate{grants: g, log: h.log}
	return h
}

/*
reportOf names the report a schedule runs, for the grant check.

Empty where the schedule is not this project's or does not exist, which the
caller reads as "nothing to check" — Fire answers for that case a moment later
and its answer is the one somebody should see.
*/
func (h *Schedules) reportOf(project *Project, name string) string {
	if project == nil || project.Definitions == nil {
		return ""
	}
	for _, s := range project.Definitions.Schedules() {
		if s.Name == name {
			return s.Report
		}
	}
	return ""
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

	/*
	   The report behind the schedule, before it renders anything.

	   A schedule is a report on a timer, so running one on demand is a way to
	   read it — and it consulted no grant at all. Checked here rather than in
	   the burst because the burst runs as the schedule's owner and has no
	   caller to judge.
	*/
	if report := h.reportOf(project, name); report != "" {
		if allowed, known := h.gate.may(r.Context(), pr, report); !known {
			fail(w, http.StatusServiceUnavailable, "Could not check who may open this report.")
			return
		} else if !allowed {
			audit(r.Context(), h.log, pr, ActionRead, report, Refused,
				map[string]any{"reason": "not granted", "via": "schedule.run", "schedule": name})
			fail(w, http.StatusNotFound, "No such schedule.")
			return
		}
	}

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
