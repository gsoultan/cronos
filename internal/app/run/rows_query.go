package run

import (
	"context"
	"fmt"

	"github.com/gsoultan/cronos/internal/core/principal"
)

// Rows reads a dataset whole, as maps.
//
// The one place a result set becomes a slice rather than a stream, and it is
// deliberate: this reads the *recipients* of a burst, not their data. A list
// of five thousand customer ids is a few hundred kilobytes, and holding it is
// what lets the fan-out be bounded — streaming recipients would mean either an
// unbounded number of renders in flight or a cursor held open for the length
// of the run.
func (s *Service) Rows(ctx context.Context, dataset string, params map[string]any,
	pr principal.Principal) ([]map[string]any, error) {

	ds, err := s.datasets.Dataset(ctx, dataset)
	if err != nil {
		return nil, err
	}
	plan, err := s.builder.Build(ds, params, pr)
	if err != nil {
		return nil, err
	}
	result, err := s.exec.Execute(ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrExecute, err)
	}
	defer result.Close()

	cols, err := result.Columns()
	if err != nil {
		return nil, err
	}

	var out []map[string]any
	for result.Next() {
		cells := make([]any, len(cols))
		into := make([]any, len(cols))
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := result.Scan(into...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, name := range cols {
			row[name] = cells[i]
		}
		out = append(out, row)
	}
	return out, result.Err()
}
