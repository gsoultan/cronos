package vault

import "time"

// Source is where a secret's value comes from today.
type Source string

const (
	// FromProject is stored here, through the portal or the API.
	FromProject Source = "project"
	// FromDeployment is the deployment's own — an environment variable or a
	// mounted file — and is changed wherever the deployment is configured.
	FromDeployment Source = "deployment"
	// Missing is named by a definition and answered by nothing, so whatever
	// reads it does not work.
	Missing Source = "missing"
)

// Entry is one secret as a person managing them sees it. Never the value.
type Entry struct {
	Name      string     `json:"name"`
	Source    Source     `json:"source"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	UpdatedBy string     `json:"updatedBy,omitempty"`
	// UsedBy is every definition naming it, so deleting one says what breaks
	// and a missing one says what is broken.
	UsedBy []Use `json:"usedBy"`
}
