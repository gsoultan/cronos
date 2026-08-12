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

	// Org and Project are where they act. One of each for now: a person in
	// several projects needs a picker and a membership table, and inventing
	// half of that would be worse than not having it.
	Org     string `json:"org"`
	Project string `json:"project"`
	Role    string `json:"role"`

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
