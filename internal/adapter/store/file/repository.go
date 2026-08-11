package file

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/core/definition"
)

// Repository holds every definition under a directory, read once.
//
// Read once rather than per request: definitions change when someone deploys,
// not while a report is running, and re-reading would make a burst of five
// thousand documents five thousand directory walks.
type Repository struct {
	datasets map[string]definition.Dataset
	reports  map[string]definition.Report
}

// ErrNotFound means no definition of that kind has that name.
var ErrNotFound = fmt.Errorf("file: no such definition")

// Load reads every .yaml under dir.
//
// It fails on the first bad file rather than skipping it. A repository that
// silently drops a definition it could not parse is one where a report
// disappears and the only evidence is a line in a startup log nobody read.
func Load(dir string) (*Repository, error) {
	r := &Repository{
		datasets: map[string]definition.Dataset{},
		reports:  map[string]definition.Report{},
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isYAML(path) {
			return err
		}
		return r.add(path)
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func (r *Repository) add(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	kind, err := codec.Loader{}.Kind(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	switch kind {
	case codec.KindDataset:
		ds, err := codec.Loader{}.Dataset(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		r.datasets[ds.Name] = ds
	case codec.KindReport:
		rep, err := codec.Loader{}.Report(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		r.reports[rep.Name] = rep
	}
	// DataSource and Schedule are read by other parts of the system; ignoring
	// them here is not the same as dropping them silently, because Kind
	// already proved the document is one of the four.
	return nil
}

// Dataset implements run.Datasets.
func (r *Repository) Dataset(_ context.Context, name string) (definition.Dataset, error) {
	ds, ok := r.datasets[name]
	if !ok {
		return definition.Dataset{}, fmt.Errorf("%w: dataset %q", ErrNotFound, name)
	}
	return ds, nil
}

// Report returns the named report.
//
// Names are keys in a map, never path fragments. A caller asking for
// "../../etc/passwd" gets ErrNotFound rather than a file, because nothing here
// joins a caller's string onto a path — the traversal is impossible rather
// than filtered.
func (r *Repository) Report(_ context.Context, name string) (definition.Report, error) {
	rep, ok := r.reports[name]
	if !ok {
		return definition.Report{}, fmt.Errorf("%w: report %q", ErrNotFound, name)
	}
	return rep, nil
}

// Counts reports what was loaded, for a startup line an operator can read.
func (r *Repository) Counts() (datasets, reports int) {
	return len(r.datasets), len(r.reports)
}
