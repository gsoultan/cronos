package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"

	"strings"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// OrgProjects is what this handler may ask about an organization, and about
// where one person belongs inside it.
type OrgProjects interface {
	ProjectsInOrg(ctx context.Context, org string) ([]string, error)
	// MembershipIn is the role somebody holds in one project, if any. Asked
	// per request rather than filtered out of a list, because the answer
	// decides whether to admit them.
	MembershipIn(ctx context.Context, userID, org, project string) (string, bool, error)
	MembershipsOf(ctx context.Context, userID string) ([]identity.Membership, error)
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
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
		return
	}
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	if r.Method == http.MethodGet {
		h.list(w, r, pr)
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
	/*
	   Two ways in, and they grant different things.

	   A member enters a project they belong to, with the role that membership
	   gives them — read from the store on this request, never carried over
	   from the session they are in. An editor in finance who is a viewer in
	   ops arrives in ops as a viewer, and the way that goes wrong is to reuse
	   the role already in hand.

	   An organization owner or admin enters any project in the organization
	   with no project role at all, because effective() promotes the org role
	   and a project grant left beside it would outlive the revocation of the
	   org one.
	*/
	role, member, err := h.projects.MembershipIn(r.Context(), pr.Subject, pr.OrgID, in.Project)
	if err != nil {
		h.log.Error("could not read a membership", "err", err)
		fail(w, http.StatusInternalServerError, "Could not do that.")
		return
	}
	if !member {
		if !pr.CanAdminOrg() {
			// 404 and not 403, for the same reason the administrator's path
			// gives one: a project they cannot enter and a project that does
			// not exist are the same thing from here, and telling them apart
			// enumerates an organization's projects for anybody with a login.
			fail(w, http.StatusNotFound, "No such project you can enter.")
			return
		}
		projects, err := h.projects.ProjectsInOrg(r.Context(), pr.OrgID)
		if err != nil {
			h.log.Error("could not list projects", "err", err)
			fail(w, http.StatusInternalServerError, "Could not do that.")
			return
		}
		if !slices.Contains(projects, in.Project) {
			fail(w, http.StatusNotFound, "No such project in this organization.")
			return
		}
		role = ""
	}

	issued, err := h.signer.Mint(token.Claims{
		Audience: token.Portal,
		Role:     role,
		OrgRole:  string(pr.OrgRole),
		Org:      pr.OrgID, Project: in.Project, Subject: pr.Subject,
		Platform: pr.Platform,
	}, SessionLifetime)
	if err != nil {
		h.log.Error("could not mint a session", "err", err)
		fail(w, http.StatusInternalServerError, "Could not do that.")
		return
	}

	// What they arrived as, and how. An audit line saying only the org role
	// would describe a member's move as an administrator's.
	how := "membership"
	granted := role
	if !member {
		how, granted = "organization role", string(pr.OrgRole)
	}
	h.log.Info("entered a project", "user", pr.Subject,
		"project", pr.OrgID+"/"+in.Project, "via", how, "role", granted)
	audit(r.Context(), h.log, pr, ActionEnterProject, in.Project, Allowed,
		map[string]any{"role": granted, "via": how})
	/*
	   The role as well as the token, because they are not the same question
	   and the caller cannot answer the second one.

	   A person is an editor in one project and a viewer in the next; the
	   token carries whichever this is, and a browser that guessed from the
	   entry somebody clicked would be reading a list it fetched some minutes
	   ago. Empty where they arrive on an organisation role and hold no
	   membership — which is not "no access", it is the state the org role is
	   read alongside.
	*/
	send(w, http.StatusOK, map[string]string{
		"token": issued, "project": in.Project, "role": role,
	})
}

/*
list is where this session may go, which is what a switcher needs to offer.

Their memberships, and for an organization owner or admin every project in the
organization — the same two ways in that the POST admits, answered before
somebody picks rather than after. A switcher that offers what the endpoint then
refuses is worse than no switcher.

The role is on each entry because it is not the same everywhere: an editor in
one project and a viewer in another should be able to see which they are about
to become.
*/
func (h *EnterProject) list(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	type entry struct {
		Project string `json:"project"`
		Role    string `json:"role"`
		Via     string `json:"via"`
	}

	mine, err := h.projects.MembershipsOf(r.Context(), pr.Subject)
	if err != nil {
		h.log.Error("could not list memberships", "err", err)
		fail(w, http.StatusInternalServerError, "Could not do that.")
		return
	}

	out := make([]entry, 0, len(mine))
	seen := map[string]bool{}
	for _, m := range mine {
		// Their own organization only. A membership in another one is not
		// reachable from this session, and offering it would be a switcher
		// that changes which organization somebody is in by accident.
		if m.Org != pr.OrgID {
			continue
		}
		out = append(out, entry{Project: m.Project, Role: m.Role, Via: "membership"})
		seen[m.Project] = true
	}

	if pr.CanAdminOrg() {
		all, err := h.projects.ProjectsInOrg(r.Context(), pr.OrgID)
		if err != nil {
			h.log.Error("could not list projects", "err", err)
			fail(w, http.StatusInternalServerError, "Could not do that.")
			return
		}
		for _, p := range all {
			if seen[p] {
				// A membership says more than the org role does: it names a
				// role, and it survives the org role being taken away.
				continue
			}
			out = append(out, entry{Project: p, Via: "organization role"})
		}
	}

	slices.SortFunc(out, func(a, b entry) int { return strings.Compare(a.Project, b.Project) })
	send(w, http.StatusOK, map[string]any{"projects": out, "current": pr.ProjectID})
}
