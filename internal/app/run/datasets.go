package run

import (
	"context"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// Datasets resolves a dataset by name, within the caller's project.
//
// The project is not a parameter: it is on the principal, and a repository
// that took it separately would let a caller ask for one project's dataset
// while acting in another. See docs/tenancy.md — project isolation is
// structural, which means it is not a field anyone can pass.
type Datasets interface {
	Dataset(ctx context.Context, name string) (definition.Dataset, error)
}
