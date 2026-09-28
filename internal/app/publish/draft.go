package publish

import (
	"context"
	"fmt"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Draft decodes a report and checks it as publishing it would, and stores
// nothing: a draft the builder draws is refused exactly where publishing it
// would be, with the same sentence, by the same bar — somebody who may change
// the project's definitions.
func (s *Service) Draft(ctx context.Context, raw []byte, pr principal.Principal) (definition.Report, error) {
	if !pr.CanEdit() {
		return definition.Report{}, fmt.Errorf("%w: %s may not change definitions", ErrForbidden, pr.ProjectRole)
	}
	rep, err := codec.Loader{}.Report(raw)
	if err != nil {
		return definition.Report{}, err
	}
	if err := s.checkReport(ctx, rep); err != nil {
		return definition.Report{}, err
	}
	return rep, nil
}
