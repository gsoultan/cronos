package file

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/core/definition"
)

// Repository holds every definition under a directory, read once.
//
// Read once rather than per request: definitions change when someone deploys,
// not while a report is running, and re-reading would make a burst of five
// thousand documents five thousand directory walks.
type Repository struct {
	// mu guards a swap after a publish. Reads are far more common than writes,
	// and a request that started before a publish should finish against a
	// consistent view rather than half of each.
	mu       sync.RWMutex
	datasets map[string]definition.Dataset
	reports  map[string]definition.Report
	// paths remembers where each definition was read from, keyed kind/name.
	//
	// A definitions directory is somebody's git repository and they organised
	// it: publishing an update to finance/invoices.yaml must overwrite that
	// file, not leave it there and create a second one under datasets/.
	paths map[string]string
}

// replace swaps in a freshly loaded view, so a running server serves what was
// just published rather than what it read at startup.
func (r *Repository) replace(fresh *Repository) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.datasets, r.reports, r.paths = fresh.datasets, fresh.reports, fresh.paths
}

// Path returns where a definition was read from.
func (r *Repository) Path(kind, name string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.paths[kind+"/"+name]
	return p, ok
}

// Names lists what is loaded, so a writer can report the repository's contents
// rather than guessing from a directory layout it does not control.
func (r *Repository) Names(kind string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []string
	for key := range r.paths {
		if k, name, ok := strings.Cut(key, "/"); ok && k == kind {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
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
		paths:    map[string]string{},
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Dot directories are skipped whole. .versions/ holds every historical
		// copy of every definition, and walking into it would load them all as
		// live — the same name several times, last one winning by directory
		// order. .git is the other one nobody means to publish.
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != dir {
				return fs.SkipDir
			}
			return nil
		}
		if !isYAML(path) {
			return nil
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
		r.paths[kind+"/"+ds.Name] = path
	case codec.KindReport:
		rep, err := codec.Loader{}.Report(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		r.reports[rep.Name] = rep
		r.paths[kind+"/"+rep.Name] = path
	}
	// DataSource and Schedule are read by other parts of the system; ignoring
	// them here is not the same as dropping them silently, because Kind
	// already proved the document is one of the four.
	return nil
}

// Dataset implements run.Datasets.
func (r *Repository) Dataset(_ context.Context, name string) (definition.Dataset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

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
	r.mu.RLock()
	defer r.mu.RUnlock()

	rep, ok := r.reports[name]
	if !ok {
		return definition.Report{}, fmt.Errorf("%w: report %q", ErrNotFound, name)
	}
	return rep, nil
}

// Counts reports what was loaded, for a startup line an operator can read.
func (r *Repository) Counts() (datasets, reports int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.datasets), len(r.reports)
}
