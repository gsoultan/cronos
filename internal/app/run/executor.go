package run

import (
	"context"

	"github.com/gsoultan/cronos/internal/core/query"
)

// Executor runs a compiled plan.
//
// It takes a query.Plan and not a string, which is the enforcement: a Plan's
// SQL is unexported and only the builder sets it, so there is no way to hand
// an executor a statement that skipped row scope. The port cannot be misused
// because the type it accepts cannot be forged.
type Executor interface {
	Execute(ctx context.Context, p query.Plan) (Rows, error)
}
