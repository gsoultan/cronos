package publish

import (
	"context"
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// Templates resolves a template a report's header or footer names.
type Templates interface {
	Template(ctx context.Context, name string) (definition.Template, error)
}

// WithTemplates lets a report be checked against the templates it draws with.
// Without it a report naming one is refused, since nothing could say whether
// it exists.
func (s *Service) WithTemplates(t Templates) *Service {
	s.templates = t
	return s
}

// checkFurniture proves every header and footer template a report names
// exists and defines the function its role calls. A letterhead used as a
// footer otherwise fails the render, which for a schedule is at the hour it
// fires and for every recipient.
func (s *Service) checkFurniture(ctx context.Context, rep definition.Report) error {
	for _, out := range rep.Outputs {
		for _, use := range []struct {
			role string
			f    definition.Furniture
		}{{"header", out.Header}, {"footer", out.Footer}} {
			role, f := use.role, use.f
			if f.Template == "" {
				continue
			}
			if s.templates == nil {
				return fmt.Errorf("%w: report %q %s draws with template %q, and this "+
					"deployment has no templates", ErrNotFound, rep.Name, role, f.Template)
			}
			t, err := s.templates.Template(ctx, f.Template)
			if err != nil {
				return fmt.Errorf("%w: report %q %s draws with template %q: %v",
					ErrNotFound, rep.Name, role, f.Template, err)
			}
			if !t.Defines(role) {
				return fmt.Errorf("%w: template %q is the %s of report %q output %q, "+
					"and it defines no %s(ctx)", definition.ErrInvalid, t.Name, role,
					rep.Name, out.Name, role)
			}
		}
	}
	return nil
}
