package vault

import "time"

// Stored is one secret as the store holds it: sealed, and who set it when.
type Stored struct {
	Org, Project, Name string
	// Sealed is empty in a listing, which has no use for it.
	Sealed    []byte
	UpdatedAt time.Time
	UpdatedBy string
}
