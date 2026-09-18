package identity

import "time"

// User is somebody who signs in to the portal.
//
// The password is never here. A struct holding a hash gets logged, serialised
// into an error, and returned from an API by somebody adding a field — so the
// hash lives in the store and comes out only for the one comparison that needs
// it.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`

	// Org and Project are where the account was created, and where a session
	// starts. Not the only place they may act: memberships say where else, and
	// a session moves between them by minting a new token rather than by
	// widening this one. See Membership.
	Org     string `json:"org"`
	Project string `json:"project"`
	Role    string `json:"role"`
	/*
	   OrgRole is what they administer across the organization, if anything.

	   Separate from Role and not a wider spelling of it. `admin` means one
	   thing about a project and another about an organization, and they are
	   the same string — one field holding both would make a project admin an
	   org admin on the first line that forgot which it was reading.

	   Empty for every account in a deployment that has never granted one,
	   which is every deployment until somebody does.
	*/
	OrgRole string `json:"orgRole,omitempty"`

	CreatedAt time.Time  `json:"createdAt"`
	LastSeen  *time.Time `json:"lastSeen,omitempty"`
	// Disabled keeps the row while refusing the login. Deleting somebody who
	// has run reports would orphan every run record that names them.
	Disabled bool `json:"disabled,omitempty"`

	/*
	   Platform marks a deployment administrator.

	   Not a role — it is orthogonal to the one above. Somebody can administer
	   the deployment and be a viewer in the only project they belong to, and
	   that is the ordinary shape: the person who runs the servers is rarely the
	   person who writes the reports.

	   It grants nothing inside a project. See principal.Principal.Platform.
	*/
	Platform bool `json:"platform,omitempty"`
}

// Tenant is one organisation and project, and how many people are in it.
//
// For platform administration, which is the only place a count across tenants
// means anything: inside a project the answer is always "this one".
type Tenant struct {
	Org      string `json:"org"`
	Project  string `json:"project"`
	People   int    `json:"people"`
	Disabled int    `json:"disabled"`
}

/*
Membership is a project somebody belongs to, and what they may do there.

A table rather than more columns on the account, because the email on an
account is unique across the deployment: somebody working in two projects
needed two accounts with two addresses, which is a workaround an administrator
invents once and then maintains for ever.

The role travels with the membership. An editor in one project and a viewer in
another is the ordinary case, and one role on the account would make the wider
of the two apply in both.
*/
type Membership struct {
	Org     string `json:"org"`
	Project string `json:"project"`
	Role    string `json:"role"`
}
