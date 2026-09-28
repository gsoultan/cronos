package vault

import "context"

// Resealing is what rotating the secrets key needs of the store: every sealed
// value, across every project, and a way to replace one only if nobody changed
// it in the meantime.
type Resealing interface {
	AllSecrets(ctx context.Context) ([]Stored, error)
	Reseal(ctx context.Context, s Stored, was []byte) (bool, error)
}
