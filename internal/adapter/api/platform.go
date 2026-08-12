package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
Administering the deployment rather than a project.

Every other handler in this package is scoped by the caller's organisation and
project, and that scoping is what keeps one customer out of another's. These
routes are deliberately not scoped, which makes them the most dangerous file
here — so the permission check happens once, at the top of ServeHTTP, before
anything is decoded or dispatched, and nothing below it re-derives who is
asking.

What is absent is as important as what is here. There is no route that reads a
report, runs a query, or returns a row of anybody's data. A platform
administrator adds accounts, moves people between projects, and sees which
tenants exist; support that needs to see what a customer sees adds itself to
that project, and the audit log records that it did. The boundary is why a
leaked platform credential is a control-plane problem rather than every
customer's warehouse at once.
*/

// Platform is what the API may ask about the deployment as a whole.
type Platform interface {
	EveryPerson(ctx context.Context) ([]identity.User, error)
	Tenants(ctx context.Context) ([]identity.Tenant, error)
	MovePerson(ctx context.Context, id, org, project, role string) error
	DisableAnywhere(ctx context.Context, id string, disabled bool) error

	PlatformAdmins(ctx context.Context) ([]identity.User, error)
	GrantPlatform(ctx context.Context, id, by string) error
	RevokePlatform(ctx context.Context, id string) error
}

// PlatformAPI serves /v1/platform.
type PlatformAPI struct {
	store Platform
	auth  Principals
	log   *slog.Logger
}

// NewPlatformAPI wires the handler.
func NewPlatformAPI(s Platform, a Principals, log *slog.Logger) *PlatformAPI {
	return &PlatformAPI{store: s, auth: a, log: log}
}

/*
ServeHTTP checks the permission once and then dispatches.

Once, and here, because the routes below read across every tenant. A check
repeated per handler is a check somebody forgets to add to the next handler, and
the failure mode of forgetting is one customer's administrator listing another's
accounts.
*/
func (h *PlatformAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	if !pr.CanAdminPlatform() {
		/*
		   404, not 403.

		   Everywhere else in this API a 403 is the honest answer, because the
		   caller already knows the resource exists — they are looking at their
		   own project. Here they are not: "you may not administer the platform"
		   confirms to somebody probing that there is a platform tier to attack,
		   and which accounts might hold it. There is nothing here for them, and
		   saying so is enough.
		*/
		fail(w, http.StatusNotFound, "No such endpoint.")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/platform")
	id := r.PathValue("id")

	switch {
	case path == "/tenants" && r.Method == http.MethodGet:
		h.tenants(w, r)
	case path == "/people" && r.Method == http.MethodGet:
		h.people(w, r)
	case strings.HasPrefix(path, "/people/") && r.Method == http.MethodPatch:
		h.amend(w, r, pr, id)
	case path == "/admins" && r.Method == http.MethodGet:
		h.admins(w, r)
	case strings.HasPrefix(path, "/admins/") && r.Method == http.MethodPost:
		h.grant(w, r, pr, id)
	case strings.HasPrefix(path, "/admins/") && r.Method == http.MethodDelete:
		h.revoke(w, r, pr, id)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

// tenants is which organisations and projects have people in them.
func (h *PlatformAPI) tenants(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.Tenants(r.Context())
	if err != nil {
		h.log.Error("could not read the tenants", "err", err)
		fail(w, http.StatusInternalServerError, "Could not read the tenants.")
		return
	}
	send(w, http.StatusOK, map[string]any{"tenants": out})
}

// people is every account, in every project.
func (h *PlatformAPI) people(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.EveryPerson(r.Context())
	if err != nil {
		h.log.Error("could not read every account", "err", err)
		fail(w, http.StatusInternalServerError, "Could not read the accounts.")
		return
	}
	send(w, http.StatusOK, map[string]any{"people": out})
}

type platformChange struct {
	Org      string `json:"org,omitempty"`
	Project  string `json:"project,omitempty"`
	Role     string `json:"role,omitempty"`
	Disabled *bool  `json:"disabled,omitempty"`
}

/*
amend moves somebody, or turns their access off, in any project.

Moving between organisations is the one thing an ordinary administrator must
never be able to do — it is how somebody would add themselves to a customer's
project — so it lives here and nowhere else.
*/
func (h *PlatformAPI) amend(w http.ResponseWriter, r *http.Request,
	pr principal.Principal, id string) {

	var in platformChange
	if err := decodeJSON(w, r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Send a project, a role or disabled.")
		return
	}

	if in.Disabled != nil {
		if id == pr.Subject && *in.Disabled {
			// Turning off your own access as a platform administrator is how a
			// deployment ends up with an administrator who cannot sign in.
			fail(w, http.StatusConflict, "You cannot turn off your own access here.")
			return
		}
		if err := h.store.DisableAnywhere(r.Context(), id, *in.Disabled); err != nil {
			h.refuse(w, err)
			return
		}
		action := ActionPlatformEnable
		if *in.Disabled {
			action = ActionPlatformDisable
		}
		audit(r.Context(), h.log, pr, action, id, Allowed, nil)
	}

	if in.Org != "" || in.Project != "" || in.Role != "" {
		if in.Org == "" || in.Project == "" {
			// Half a move would leave somebody in an organisation with a
			// project that does not belong to it.
			fail(w, http.StatusBadRequest, "Moving somebody needs both an organisation and a project.")
			return
		}
		if in.Role != "" && !validRole(in.Role) {
			fail(w, http.StatusBadRequest, "A role is admin, editor or viewer.")
			return
		}
		role := in.Role
		if role == "" {
			role = "viewer"
		}
		if err := h.store.MovePerson(r.Context(), id, in.Org, in.Project, role); err != nil {
			h.refuse(w, err)
			return
		}
		h.log.Warn("account moved between projects",
			"user", id, "to", in.Org+"/"+in.Project, "role", role, "by", pr.Subject)
		audit(r.Context(), h.log, pr, ActionPlatformMove, id, Allowed,
			map[string]any{"org": in.Org, "project": in.Project, "role": role})
	}

	w.WriteHeader(http.StatusNoContent)
}

// admins lists who administers the deployment.
func (h *PlatformAPI) admins(w http.ResponseWriter, r *http.Request) {
	out, err := h.store.PlatformAdmins(r.Context())
	if err != nil {
		h.log.Error("could not read the platform administrators", "err", err)
		fail(w, http.StatusInternalServerError, "Could not read the administrators.")
		return
	}
	send(w, http.StatusOK, map[string]any{"admins": out})
}

func (h *PlatformAPI) grant(w http.ResponseWriter, r *http.Request,
	pr principal.Principal, id string) {

	if err := h.store.GrantPlatform(r.Context(), id, pr.Subject); err != nil {
		h.refuse(w, err)
		return
	}
	// At warning level, like removing a second factor: it is rare, it widens
	// somebody's reach across every tenant, and it is invisible afterwards.
	h.log.Warn("platform administrator granted", "user", id, "by", pr.Subject)
	audit(r.Context(), h.log, pr, ActionPlatformGrant, id, Allowed, nil)
	w.WriteHeader(http.StatusNoContent)
}

/*
revoke takes it away, and ends that account's sessions.

The claim is in the token so that every request does not have to ask the
database, which means it outlives the grant until the token expires. The store
cuts their sessions in the same transaction, so what a revoked administrator
experiences is being signed out rather than keeping the reach for eight hours.
*/
func (h *PlatformAPI) revoke(w http.ResponseWriter, r *http.Request,
	pr principal.Principal, id string) {

	if err := h.store.RevokePlatform(r.Context(), id); err != nil {
		if errors.Is(err, sqlstore.ErrLastPlatformAdmin) {
			fail(w, http.StatusConflict,
				"This is the last platform administrator. Grant it to somebody else first — "+
					"a deployment with none cannot make another except from the command line.")
			return
		}
		h.refuse(w, err)
		return
	}

	h.log.Warn("platform administrator revoked", "user", id, "by", pr.Subject)
	audit(r.Context(), h.log, pr, ActionPlatformRevoke, id, Allowed, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *PlatformAPI) refuse(w http.ResponseWriter, err error) {
	if errors.Is(err, identity.ErrNoUser) {
		fail(w, http.StatusNotFound, "No such account.")
		return
	}
	if errors.Is(err, identity.ErrBadCredentials) {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	h.log.Error("platform administration", "err", err)
	fail(w, http.StatusInternalServerError, "Could not do that.")
}
