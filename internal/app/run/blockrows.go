package run

import (
	"context"
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// ExportLimit caps a spreadsheet export.
//
// Far larger than a table block's page, because an export exists to be opened
// in something else and truncating at a hundred rows would make it useless.
// Not unbounded, because a million-row XLSX is a file that crashes the
// application it was exported for, and a report engine that produces one has
// helped nobody.
const ExportLimit = 100_000

// BlockRows runs a block and returns the values a database gave, untouched.
//
// The formatted path is the usual one — every renderer wants strings, and
// formatting once keeps a figure the same everywhere. A spreadsheet is the
// exception that justifies this second path: a total written as text is a
// total nobody can sum in the tool they opened it in, which is the entire
// reason they asked for a spreadsheet rather than a PDF.
func (s *Service) BlockRows(ctx context.Context, ds definition.Dataset, blk definition.Block,
	params map[string]any, filters query.Filters, pr principal.Principal) ([]string, [][]any, error) {

	engine, err := s.engines.Engine(ctx, ds)
	if err != nil {
		return nil, nil, err
	}
	plan, _, err := engine.Builder.BuildBlock(ds, blk, params, filters, pr)
	if err != nil {
		return nil, nil, err
	}
	result, err := engine.Executor.Execute(ctx, plan)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrExecute, err)
	}
	defer result.Close()

	cols, err := result.Columns()
	if err != nil {
		return nil, nil, err
	}

	var out [][]any
	for result.Next() {
		cells := make([]any, len(cols))
		into := make([]any, len(cols))
		for i := range cells {
			into[i] = &cells[i]
		}
		if err := result.Scan(into...); err != nil {
			return nil, nil, err
		}
		out = append(out, cells)
	}
	return cols, out, result.Err()
}
