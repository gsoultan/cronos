package api

import (
	"context"
	"log/slog"
	"net/http"

	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
What a project requires of the people in it.

One switch, and the panel that showed it for a year showed it over sample data —
so an administrator could turn on "require two-factor" and nothing whatever
would happen. It was gated to sample mode rather than shipped half-built,
because the flag was never the hard part.

The hard part was what happens to somebody who has none: they cannot enrol
without signing in and cannot sign in without enrolling. Refusing the sign-in
locks a team out of its own reporting on the afternoon somebody switches this
on, with no self-service way back — which is also the moment an administrator
gets a phone call asking them to turn a second factor off, the exact call the
second factor exists to make suspicious.

So they sign in, to a session that reaches the enrolment endpoints and nothing
else. Nobody is locked out, and the requirement bites on the next sign-in rather
than on a deadline somebody misses while on leave.
*/

// Policies is what the API may ask and say about a project's requirements.
type Policies interface {
	PolicyOf(ctx context.Context, org, project string) (sqlstore.Policy, error)
	SetPolicy(ctx context.Context, pr principal.Principal, p sqlstore.Policy) error
	Covered(ctx context.Context, pr principal.Principal) (with, without int, err error)
}

// PolicyAPI serves /v1/policy.
type PolicyAPI struct {
	store Policies
	auth  Principals
	log   *slog.Logger
}

// NewPolicyAPI wires the handler.
func NewPolicyAPI(p Policies, a Principals, log *slog.Logger) *PolicyAPI {
	return &PolicyAPI{store: p, auth: a, log: log}
}

// ServeHTTP handles GET and PUT /v1/policy.
func (h *PolicyAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Readable by anybody in the project. What it says is "you will be
		// asked for a second factor", which is a thing to be told rather than
		// to discover at the next sign-in.
		h.show(w, r, pr)
	case http.MethodPut:
		if !pr.CanAdminProject() {
			fail(w, http.StatusForbidden, "Only a project administrator may change this.")
			return
		}
		h.set(w, r, pr)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

func (h *PolicyAPI) show(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	policy, err := h.store.PolicyOf(r.Context(), pr.OrgID, pr.ProjectID)
	if err != nil {
		h.log.Error("could not read the policy", "err", err)
		fail(w, http.StatusInternalServerError, "Could not read the policy.")
		return
	}

	out := map[string]any{"requireTwoFactor": policy.RequireTwoFactor}

	// The counts only where somebody can act on them. A viewer being told how
	// many colleagues have no second factor is a list of who to attack.
	if pr.CanAdminProject() {
		with, without, err := h.store.Covered(r.Context(), pr)
		if err != nil {
			h.log.Error("could not count who is covered", "err", err)
		} else {
			out["covered"], out["uncovered"] = with, without
		}
	}
	send(w, http.StatusOK, out)
}

/*
set changes what the project requires.

Turning it on is deliberately not guarded by "are you covered yourself?" — the
older design of this panel refused an administrator who had no factor, because
under a lockout rollout they would be the first person locked out and then
nobody could turn it back off. With an enrolment-only session that cannot
happen: they sign in, they enrol, they carry on. The guard would now be
friction protecting against nothing.
*/
func (h *PolicyAPI) set(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	var in struct {
		RequireTwoFactor bool `json:"requireTwoFactor"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Send requireTwoFactor.")
		return
	}

	policy := sqlstore.Policy{RequireTwoFactor: in.RequireTwoFactor}
	if err := h.store.SetPolicy(r.Context(), pr, policy); err != nil {
		h.log.Error("could not set the policy", "err", err)
		fail(w, http.StatusInternalServerError, "Could not save that.")
		return
	}

	h.log.Warn("project policy changed",
		"project", pr.OrgID+"/"+pr.ProjectID,
		"requireTwoFactor", in.RequireTwoFactor, "by", pr.Subject)
	audit(r.Context(), h.log, pr, ActionPolicySet, pr.OrgID+"/"+pr.ProjectID, Allowed,
		map[string]any{"requireTwoFactor": in.RequireTwoFactor})
	send(w, http.StatusOK, map[string]any{"requireTwoFactor": in.RequireTwoFactor})
}
