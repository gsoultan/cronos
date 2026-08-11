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

// effective resolves the project-level role, accounting for org administrators
// who hold no explicit project membership. An org without an owner who can fix
// a broken report grows a back door instead.
func (p Principal) effective() Role {
	if p.OrgRole == OrgOwner || p.OrgRole == OrgAdmin {
		return ProjectAdmin
	}
	return p.ProjectRole
}
