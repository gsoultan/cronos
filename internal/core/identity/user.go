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
}
