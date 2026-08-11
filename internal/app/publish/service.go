package publish

import (
	"context"
	"fmt"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// Datasets resolves a dataset the document under review refers to.
//
// Needed because a report cannot be checked alone: whether a filter binds to a
// real field is a question about the dataset, and the answer is the difference
// between a report that renders and one that fails on first open.
type Datasets interface {
	Dataset(ctx context.Context, name string) (definition.Dataset, error)
}

// Service validates and stores definitions.
type Service struct {
	store    Store
	datasets Datasets
}

// New wires a Service.
func New(s Store, d Datasets) *Service { return &Service{store: s, datasets: d} }

// Publish validates raw and stores it.
func (s *Service) Publish(ctx context.Context, raw []byte, pr principal.Principal) (Result, error) {
	if !pr.CanEdit() {
		return Result{}, fmt.Errorf("%w: %s may not change definitions", ErrForbidden, pr.ProjectRole)
	}

	kind, err := codec.Loader{}.Kind(raw)
	if err != nil {
		return Result{}, err
	}

	name, err := s.check(ctx, kind, raw)
	if err != nil {
		return Result{}, err
	}

	version, err := s.store.Put(ctx, kind, name, raw)
	if err != nil {
		return Result{}, err
	}
	return Result{Kind: kind, Name: name, Version: version}, nil
}

// check proves the document will work, and returns the name to store it under.
//
// The name comes from the document rather than from the request path. A
// mismatch between the two is a rename someone did not mean, and taking the
// path would silently store one definition under another's name.
func (s *Service) check(ctx context.Context, kind string, raw []byte) (string, error) {
	switch kind {
	case codec.KindDataset:
		// The loader already ran definition.Validate.
		ds, err := codec.Loader{}.Dataset(raw)
		if err != nil {
			return "", err
		}
		if err := query.Check(ds); err != nil {
			return "", err
		}
		return ds.Name, nil

	case codec.KindReport:
		rep, err := codec.Loader{}.Report(raw)
		if err != nil {
			return "", err
		}
		return rep.Name, s.checkReport(ctx, rep)
	}
	return "", fmt.Errorf("%w: %s", ErrUnsupported, kind)
}

// checkReport resolves everything the report reads and checks it against them.
func (s *Service) checkReport(ctx context.Context, rep definition.Report) error {
	sets := map[string]definition.Dataset{}
	for _, name := range rep.Datasets() {
		ds, err := s.datasets.Dataset(ctx, name)
		if err != nil {
			// A report naming a dataset nobody has is a report that renders
			// nothing, and it is far cheaper to say so now than to let someone
			// discover it when a customer opens the page.
			return fmt.Errorf("%w: report %q reads dataset %q: %v",
				ErrNotFound, rep.Name, name, err)
		}
		sets[name] = ds
	}
	if err := query.CheckFilters(rep.Filters, sets); err != nil {
		return err
	}
	return s.checkBlocks(rep, sets)
}

// checkBlocks compiles every block, which is the only way to know they will.
//
// Compiling is cheaper than being wrong: it catches a field the dataset does
// not publish, an aggregate nobody implements, and a grain the dialect cannot
// express — all of which are otherwise found by whoever opens the report.
func (s *Service) checkBlocks(rep definition.Report, sets map[string]definition.Dataset) error {
	// Postgres, because it supports every grain: a check that used a narrower
	// dialect would pass definitions the eventual database cannot run, and one
	// that used a narrower one still would reject definitions that are fine.
	builder := query.NewBuilder(query.Postgres{})
	filters := query.Filters{Defs: rep.Filters}

	for _, out := range rep.Outputs {
		for i, blk := range out.Layout {
			if blk.Kind == definition.TextBlock {
				continue
			}
			ds := sets[blk.DatasetFor(rep.Dataset)]
			if _, _, err := builder.BuildBlock(ds, blk, defaults(ds), filters, checker(ds)); err != nil {
				return fmt.Errorf("%w: output %q block %d: %v", query.ErrBadTemplate, out.Name, i, err)
			}
		}
	}
	return nil
}

// defaults supplies a value for every required parameter, so compilation is
// testing the block's shape rather than whether someone remembered to pass a
// date to a validator.
func defaults(ds definition.Dataset) map[string]any {
	in := map[string]any{}
	for _, p := range ds.Params {
		if p.Required && !p.HasDefault() {
			in[p.Name] = placeholder(p)
		}
	}
	return in
}

func placeholder(p definition.Param) any {
	switch p.Type {
	case definition.Date:
		return "2000-01-01"
	case definition.Number:
		return float64(0)
	case definition.Bool:
		return false
	case definition.Enum:
		if len(p.Values) > 0 {
			if p.Multiple {
				return []any{p.Values[0]}
			}
			return p.Values[0]
		}
	}
	if p.Multiple {
		return []any{""}
	}
	return ""
}

// checker is a principal that satisfies every row scope the dataset declares,
// so compilation exercises the real predicate rather than the FALSE a
// scope-less caller would get.
func checker(ds definition.Dataset) principal.Principal {
	scope := map[string]string{}
	for _, s := range ds.RowLevelSecurity {
		for _, name := range query.ScopeKeys(s.Predicate) {
			scope[name] = "check"
		}
	}
	return principal.Principal{Subject: "publish-check", ProjectRole: principal.ProjectViewer, Scope: scope}
}
