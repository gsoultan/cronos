package publish

import (
	"context"
	"fmt"
	"sort"
	"strings"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Catalog is everything the project has, for working out what points at what.
//
// A port rather than four lookups, because the question is the other way round
// from every other one here: not "what does this definition refer to" but "who
// refers to this one", and only something holding all of them can answer it.
type Catalog interface {
	Datasets() []definition.Dataset
	Reports() []definition.Report
	Schedules() []definition.Schedule
}

// WithCatalog lets a delete say what would break.
func (s *Service) WithCatalog(c Catalog) *Service {
	s.catalog = c
	return s
}

/*
Delete removes a definition, unless something still points at it.

Through here rather than straight to the store, for two reasons. The store
checks the tenant and nothing else, so a viewer's token reached it and removed
whatever it named — the permission to change definitions lives in this service
and every path that changes one has to come through it.

And a definition is not alone. Deleting the dataset a report reads leaves the
report to fail on the next open, or at six in the morning on the first of the
month, with an error naming something that no longer exists to explain itself.
Naming the dependants is the difference between a mistake somebody can undo in
the next second and one they find out about from a customer.
*/
func (s *Service) Delete(ctx context.Context, pr principal.Principal, kind, name string) error {
	if !pr.CanEdit() {
		return fmt.Errorf("%w: %s may not delete definitions", ErrForbidden, pr.ProjectRole)
	}
	if used := s.dependants(kind, name); len(used) > 0 {
		return fmt.Errorf("%w: %s %q is still read by %s",
			ErrInUse, strings.ToLower(kind), name, english(used))
	}
	return s.store.Delete(ctx, pr, kind, name)
}

// dependants lists what would break, by name and kind.
func (s *Service) dependants(kind, name string) []string {
	if s.catalog == nil {
		return nil
	}
	var used []string
	add := func(what, who string) { used = append(used, what+" "+quoted(who)) }

	switch kind {
	case codec.KindDataSource:
		for _, ds := range s.catalog.Datasets() {
			for _, src := range ds.Sources {
				if src.Ref == name {
					add("dataset", ds.Name)
				}
			}
		}
	case codec.KindDataset:
		for _, rep := range s.catalog.Reports() {
			if reads(rep, name) {
				add("report", rep.Name)
			}
		}
		for _, sc := range s.catalog.Schedules() {
			// A burst fans out over a dataset, which is a reference the report
			// it renders knows nothing about.
			if sc.Burst != nil && sc.Burst.Over.Dataset == name {
				add("schedule", sc.Name)
			}
		}
	case codec.KindReport:
		for _, sc := range s.catalog.Schedules() {
			if sc.Report == name {
				add("schedule", sc.Name)
			}
		}
	}
	sort.Strings(used)
	return used
}

// reads reports whether a report binds to the dataset, by default or by block.
func reads(rep definition.Report, dataset string) bool {
	if rep.Dataset == dataset {
		return true
	}
	for _, o := range rep.Outputs {
		for _, b := range o.Layout {
			if b.Dataset == dataset {
				return true
			}
		}
	}
	return false
}

// english joins names the way a sentence does, because this ends up in one.
func english(names []string) string {
	switch len(names) {
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

func quoted(s string) string { return "\"" + s + "\"" }
