package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"

	"github.com/gsoultan/cronos/internal/platform/token"
)

// OrgProjects is what this handler may ask about an organization.
type OrgProjects interface {
	ProjectsInOrg(ctx context.Context, org string) ([]string, error)
}

/*
EnterProject moves an administrator's session into another project.

The half of the org role that makes it worth having. An org owner or admin may
enter any project in their organization without a membership in it — which is
what somebody needs at six in the morning when a report is broken in a project
they were never added to, and what every product lacking it grows a back door
instead of.

Nothing here widens what they may do. effective() already promotes an org owner
or admin to ProjectAdmin, so the grant was decided when the role was; all this
changes is which project the session names, and it changes it by minting a new
token rather than by trusting one the caller sends.
*/
type EnterProject struct {
	projects OrgProjects
	auth     Principals
	signer   *token.Signer
	log      *slog.Logger
}

// NewEnterProject serves /v1/auth/project.
func NewEnterProject(p OrgProjects, a Principals, s *token.Signer, log *slog.Logger) *EnterProject {
	return &EnterProject{projects: p, auth: a, signer: s, log: log}
}

func (h *EnterProject) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
		return
	}
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	/*
	   The role, not the project role.

	   A project administrator administers the project they are in. Entering
	   another one without a membership is the organization's grant, and the
	   difference matters here more than anywhere: CanAdminProject is true for
	   an org admin too, so checking it would let every project admin in the
	   deployment walk into every other project of their organization.
	*/
	if !pr.CanAdminOrg() {
		fail(w, http.StatusForbidden,
			"Only an organization owner or administrator may enter another project.")
		return
	}

	var in struct {
		Project string `json:"project"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil || in.Project == "" {
		fail(w, http.StatusBadRequest, "Name a project.")
		return
	}

	/*
	   Their own organization, and a project that is in it.

	   The org comes from the token and is never read from the body: an
	   administrator of one organization naming another's project is the one
	   thing this endpoint must not do, and taking it from the request would
	   make that a typo away.
	*/
	projects, err := h.projects.ProjectsInOrg(r.Context(), pr.OrgID)
	if err != nil {
		h.log.Error("could not list projects", "err", err)
		fail(w, http.StatusInternalServerError, "Could not do that.")
		return
	}
	if !slices.Contains(projects, in.Project) {
		// 404 rather than 403. They administer this organization, so the
		// honest answer is that it has no such project — and a 403 would
		// confirm the existence of one in somebody else's.
		fail(w, http.StatusNotFound, "No such project in this organization.")
		return
	}

	issued, err := h.signer.Mint(token.Claims{
		Audience: token.Portal,
		// Their project role in a project they are not a member of is none.
		// Everything they may do there comes from the organization role, which
		// effective() promotes — so a revocation of that role takes this away
		// as well, rather than leaving a project grant behind.
		Role:    "",
		OrgRole: string(pr.OrgRole),
		Org:     pr.OrgID, Project: in.Project, Subject: pr.Subject,
		Platform: pr.Platform,
	}, SessionLifetime)
	if err != nil {
		h.log.Error("could not mint a session", "err", err)
		fail(w, http.StatusInternalServerError, "Could not do that.")
		return
	}

	h.log.Info("entered a project", "user", pr.Subject,
		"project", pr.OrgID+"/"+in.Project, "role", pr.OrgRole)
	audit(r.Context(), h.log, pr, ActionEnterProject, in.Project, Allowed,
		map[string]any{"role": string(pr.OrgRole)})
	send(w, http.StatusOK, map[string]string{
		"token": issued, "project": in.Project,
	})
}
