package run

import (
	"context"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// Templates resolves the template a paginated output's header or footer
// names, within the caller's project — the repository behind it is the
// project's own, as it is for Datasets.
type Templates interface {
	Template(ctx context.Context, name string) (definition.Template, error)
}
