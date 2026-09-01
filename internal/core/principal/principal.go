// Package principal carries the identity and scope a request runs as.
//
// The active organization and project are resolved once, at the edge, and
// travel unchanged from there. Nothing downstream re-derives them, and nothing
// falls back to a default: a user with nine projects acts in exactly one per
// request, and inferring which is how a request reads the right report against
// the wrong project. See docs/tenancy.md.
package principal

// Role is a grant at one level. Org roles administer the account; project roles
// govern content.
type Role string

const (
	// Organization roles.
	OrgOwner  Role = "owner"
	OrgAdmin  Role = "admin"
	OrgMember Role = "member"

	// Project roles.
	ProjectAdmin  Role = "admin"
	ProjectEditor Role = "editor"
	ProjectViewer Role = "viewer"

	// None means no grant at that level.
	None Role = ""
)

// Principal is who a request is, and where it is acting.
type Principal struct {
	// Subject identifies the user, stable across sessions.
	Subject string
	Email   string

	// The active context. Both are always set for a scoped request.
	OrgID     string
	ProjectID string

	// Grants at each level. ProjectRole may be None while OrgRole is owner or
	// admin — those roles enter any project in their organization.
	OrgRole     Role
	ProjectRole Role

	// Scope carries row-level constraints from an embed token, keyed by field.
	// It only ever narrows: values here are conjoined with the dataset's own
	// row-level security, never substituted for it.
	Scope map[string]string

	// Member marks somebody acting inside the project rather than an end
	// customer of it — an author in the portal, a schedule's owner.
	//
	// Row scope does not apply to them. docs/tenancy.md sets this out: row
	// scope isolates an ISV's end customers from each other, and a project
	// member is protected by membership and project ownership, "which is
	// sufficient — project isolation is already structural". Without this an
	// author cannot preview a row-scoped report at all: they have no embed
	// token, so the predicate matches nothing and every figure is blank.
	//
	// False by default, and that direction matters. A principal nobody marked
	// is treated as an end customer and gets the fail-closed predicate, so the
	// cost of forgetting is no rows rather than everybody's.
	Member bool

	/*
	   Platform marks somebody who administers the deployment itself rather than
	   any one project: adding accounts, moving people between projects, seeing
	   which tenants a process serves.

	   Administration only, and that is the whole design. It grants nothing at
	   all inside a project — not reading a report, not running one, not editing
	   a definition. Reaching a project's data still requires membership in it.

	   The reason is what a leaked credential costs. A platform administrator
	   who could also read every project is one credential away from every
	   customer's data at once; one who cannot is a control-plane problem, which
	   is bad and is not the same thing. Support that needs to see what a
	   customer sees adds themselves to that project, and the audit log says so.
	*/
	Platform bool

	/*
	   Enrol marks a session that exists only to set up a second factor.

	   Its project requires one and this account has none. Rather than refusing
	   the sign-in — which locks a team out of its own reporting on the
	   afternoon somebody turns the requirement on — they get in, and get
	   nowhere: the enrolment endpoints and nothing else.

	   Deliberately not consulted by CanRead, CanEdit or either administrative
	   check. Those answer "what may this role do"; this answers "may this
	   session do anything at all", which is a different question asked earlier,
	   in one place. See api.OnlyEnrolment.
	*/
	Enrol bool
}

// CanRead reports whether the principal may run reports in the active project.
func (p Principal) CanRead() bool {
	return p.effective() != None
}

// CanEdit reports whether the principal may create or change definitions.
func (p Principal) CanEdit() bool {
	switch p.effective() {
	case ProjectAdmin, ProjectEditor:
		return true
	default:
		return false
	}
}

// CanAdminProject reports whether the principal may manage project membership,
// datasources and settings.
func (p Principal) CanAdminProject() bool {
	return p.effective() == ProjectAdmin
}

// CanAdminOrg reports whether the principal may manage the organization's
// members and projects.
func (p Principal) CanAdminOrg() bool {
	return p.OrgRole == OrgOwner || p.OrgRole == OrgAdmin
}

/*
CanAdminPlatform reports whether the principal may administer the deployment.

Deliberately not consulted by any of the checks above. A reader of this file
should be able to see, from the four methods that decide access to data, that
none of them mentions Platform — because the moment one does, "administration
only" becomes a sentence in a comment rather than a property of the code.
*/
func (p Principal) CanAdminPlatform() bool { return p.Platform }

// effective resolves the project-level role, accounting for org administrators
// who hold no explicit project membership. An org without an owner who can fix
// a broken report grows a back door instead.
func (p Principal) effective() Role {
	if p.OrgRole == OrgOwner || p.OrgRole == OrgAdmin {
		return ProjectAdmin
	}
	return p.ProjectRole
}
