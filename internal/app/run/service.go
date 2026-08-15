package run

import (
	"context"
	"fmt"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// Engine is how one dataset's queries are compiled and run.
//
// Both together, because they are one decision. A dataset lives in a database,
// and that database determines the SQL to write as much as it determines where
// to send it — pairing a Postgres builder with a SQLite connection produces
// statements that parse and bind the wrong values.
type Engine struct {
	Executor Executor
	Builder  query.Builder
}

// Engines resolves the engine for a dataset.
//
// A port rather than a field, because "which database does this dataset live
// in" is a deployment's answer and not this package's. A single-source
// deployment implements it with one engine and never notices.
type Engines interface {
	Engine(ctx context.Context, ds definition.Dataset) (Engine, error)
}

// Service renders reports.
type Service struct {
	datasets Datasets
	engines  Engines
}

// New wires a Service.
func New(d Datasets, e Engines) *Service {
	return &Service{datasets: d, engines: e}
}

// One is an Engines that answers with the same engine for everything.
//
// What a deployment reading a single database wants, and what the tests use.
// Written here rather than in each caller so "there is only one database" is a
// stated configuration rather than an assumption spread across the code.
type One struct{ Only Engine }

// Engine returns the one engine, whatever was asked for.
func (o One) Engine(context.Context, definition.Dataset) (Engine, error) {
	return o.Only, nil
}

// Request is what a caller asks for.
type Request struct {
	// Output names the profile to render. Empty means the first interactive
	// one, which is what an embedded viewer wants without having to know the
	// author's naming.
	Output string
	// Params are the dataset parameters the caller supplied.
	Params map[string]any
	// Filters are the report's shared filters, as sent.
	Filters map[string]query.FilterValue
}

// Render runs every block of r and returns what a viewer draws.
func (s *Service) Render(ctx context.Context, r definition.Report, req Request,
	pr principal.Principal) (View, error) {

	out, err := pick(r, req.Output)
	if err != nil {
		return View{}, err
	}

	params, err := applyOverrides(r, req.Params)
	if err != nil {
		return View{}, err
	}
	filters := query.Filters{Defs: r.Filters, Values: req.Filters}

	view := View{Title: r.Heading(), Description: r.Description, Filters: filterViews(r)}
	for _, blk := range out.Layout {
		b, err := s.block(ctx, r, blk, params, filters, pr)
		if err != nil {
			return View{}, err
		}
		view.Blocks = append(view.Blocks, b)
	}
	return view, nil
}

func pick(r definition.Report, name string) (definition.Output, error) {
	if name != "" {
		out, ok := r.Output(name)
		if !ok {
			return definition.Output{}, fmt.Errorf("%w: report %q has no output %q",
				ErrNotRenderable, r.Name, name)
		}
		// A spreadsheet output describes sheets, not a layout, so drawing it
		// means walking an empty list and answering with a report that has no
		// blocks in it. That is what this did: two hundred, a title, and
		// nothing — for a host application asking a reasonable question by the
		// wrong route, and for the reader in front of it, neither of whom is
		// told anything. Statements and Workbook both refuse the outputs they
		// cannot produce; this is the third door and it was open.
		if out.Renderer == definition.Spreadsheet {
			return definition.Output{}, fmt.Errorf(
				"%w: output %q of report %q renders %s, which is a file rather than a view",
				ErrNotRenderable, name, r.Name, out.Renderer)
		}
		return out, nil
	}
	out, ok := r.Rendered(definition.Interactive)
	if !ok {
		return definition.Output{}, fmt.Errorf("%w: report %q has nothing a browser draws",
			ErrNotRenderable, r.Name)
	}
	return out, nil
}

// applyOverrides folds the report's own parameter narrowing over the caller's.
//
// A pinned override wins and says so. Silently ignoring it would be worse: the
// caller would see a report that does not match what they asked for and have
// no way to learn why.
func applyOverrides(r definition.Report, in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(in)+len(r.Params))
	for name, o := range r.Params {
		if o.Default != nil {
			out[name] = o.Default
		}
	}
	for name, v := range in {
		if r.Pinned(name) {
			return nil, fmt.Errorf("%w: report %q pins %q", ErrPinned, r.Name, name)
		}
		out[name] = v
	}
	return out, nil
}

func filterViews(r definition.Report) []Filter {
	out := make([]Filter, 0, len(r.Filters))
	for _, f := range r.Filters {
		out = append(out, Filter{
			Name: f.Name, Label: f.Label, Type: string(f.Type), Values: f.Values,
		})
	}
	return out
}

// block compiles and runs one block.
func (s *Service) block(ctx context.Context, r definition.Report, blk definition.Block,
	params map[string]any, filters query.Filters, pr principal.Principal) (Block, error) {

	if blk.Kind == definition.TextBlock {
		return Block{Kind: string(blk.Kind), Title: blk.Heading(), Value: blk.Text}, nil
	}

	ds, err := s.datasets.Dataset(ctx, blk.DatasetFor(r.Dataset))
	if err != nil {
		return Block{}, err
	}
	engine, err := s.engines.Engine(ctx, ds)
	if err != nil {
		return Block{}, err
	}
	plan, cov, err := engine.Builder.BuildBlock(ds, blk, params, filters, pr)
	if err != nil {
		return Block{}, err
	}
	rows, err := engine.Executor.Execute(ctx, plan)
	if err != nil {
		return Block{}, fmt.Errorf("%w: block %q: %v", ErrExecute, blk.Heading(), err)
	}
	defer rows.Close()

	return read(blk, ds, rows, coverage(cov))
}

func coverage(c query.Coverage) *Coverage {
	if len(c.Applied) == 0 && len(c.Ignored) == 0 {
		return nil
	}
	return &Coverage{Applied: c.Applied, Ignored: c.Ignored}
}
