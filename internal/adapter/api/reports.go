package api

import (
	"context"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// Reports resolves a report by name within the caller's project.
//
// Declared here because this is where it is consumed. The project is not a
// parameter: it comes from the token, and a repository that took it separately
// would let a caller name one project's report while acting in another.
type Reports interface {
	Report(ctx context.Context, name string) (definition.Report, error)
}
