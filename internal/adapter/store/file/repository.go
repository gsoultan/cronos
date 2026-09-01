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
	mu        sync.RWMutex
	datasets  map[string]definition.Dataset
	reports   map[string]definition.Report
	schedules map[string]definition.Schedule
	sources   map[string]definition.DataSource
	// raws remembers the document each definition was decoded from, keyed
	// kind/name.
	//
	// Kept rather than discarded because two things need the bytes and neither
	// can reconstruct them: the content address a run record names, and the
	// management API answering for a definition no store holds. Re-reading the
	// file for either would be a syscall per request and would answer nothing
	// at all for a definition that never came from one.
	raws map[string][]byte
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
	r.schedules, r.sources, r.raws = fresh.schedules, fresh.sources, fresh.raws
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

// empty is a repository holding nothing.
func empty() *Repository {
	return &Repository{
		datasets:  map[string]definition.Dataset{},
		reports:   map[string]definition.Report{},
		schedules: map[string]definition.Schedule{},
		sources:   map[string]definition.DataSource{},
		raws:      map[string][]byte{},
		paths:     map[string]string{},
	}
}

// Adopt replaces everything with the given documents.
//
// What a deployment does when a store is authoritative: the directory was the
// bootstrap, and from then on what runs is what the store holds — including
// definitions no file ever had, and excluding files somebody deleted through
// the API.
//
// All or nothing. A repository half-replaced by a document that failed to
// decode would be a server running some definitions from the store and the
// rest from a directory it was told not to trust.
func (r *Repository) Adopt(docs [][]byte) error {
	fresh := empty()
	for _, raw := range docs {
		if err := fresh.insert(raw, ""); err != nil {
			return err
		}
	}
	r.replace(fresh)
	return nil
}

/*
AdoptUsable takes the documents it can and returns the reasons for the rest.

The fallback when Adopt refuses, and only then — the ordinary path stays all or
nothing. What it protects against is a store holding a document this build will
not accept, which is not hypothetical: validation gets stricter, and a schedule
with a timezone nobody checked at the time it was published becomes, on the next
release, a deployment that will not start. With the API down, the only way to
remove that definition is a prompt on the database.

Which is the shape of every unrecoverable failure in this product: the fix for
the thing that is broken requires the thing that is broken. One definition of
fifty taking the other forty-nine off the air is not a trade anybody would
choose, and it is not the trade Adopt's comment was defending — that one is
about a repository half from the store and half from a directory it was told not
to trust. This is still entirely the store, minus what it could not read.

Loud rather than quiet: the caller logs each reason at error and counts them,
because a definition that silently disappeared is the failure this is not
allowed to become.
*/
func (r *Repository) AdoptUsable(docs [][]byte) []error {
	fresh := empty()
	var refused []error
	for _, raw := range docs {
		if err := fresh.insert(raw, ""); err != nil {
			refused = append(refused, err)
		}
	}
	r.replace(fresh)
	return refused
}

// Load reads every .yaml under dir.
//
// It fails on the first bad file rather than skipping it. A repository that
// silently drops a definition it could not parse is one where a report
// disappears and the only evidence is a line in a startup log nobody read.
func Load(dir string) (*Repository, error) {
	r := empty()
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
	return r.insert(data, path)
}

// Apply makes a document live without going through a file.
//
// A file-backed store rewrites the file and reloads the directory, so it needs
// none of this. A database-backed one has no file to rewrite, and without this
// a definition published through the API would sit in the store while every
// request kept being answered from what the process read at startup — visible
// in the catalogue, absent from every run, until somebody restarted the server.
//
// The origin is left empty: the document no longer comes from anywhere on
// disk, and claiming it does would make the next publish overwrite a file
// whose contents this one never had.
func (r *Repository) Apply(raw []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insert(raw, "")
}

// Raw returns the document a definition was decoded from.
//
// What lets the management API answer for a definition the running server is
// using but no store holds — a directory-bootstrapped deployment, where the
// alternative is a 404 for a report that plainly renders.
func (r *Repository) Raw(kind, name string) ([]byte, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	raw, ok := r.raws[kind+"/"+name]
	return raw, ok
}

// Version is the content address of the definition currently loaded.
//
// A run record names one so it can be replayed against exactly the bytes that
// produced it. "This report" changes; this string does not.
func (r *Repository) Version(kind, name string) (string, bool) {
	raw, ok := r.Raw(kind, name)
	if !ok {
		return "", false
	}
	return Version(raw), true
}

// insert decodes one document into the maps. The caller holds the lock, or is
// building a repository nothing else can see yet.
func (r *Repository) insert(data []byte, path string) error {
	kind, err := codec.Loader{}.Kind(data)
	if err != nil {
		return fmt.Errorf("%s: %w", origin(path), err)
	}
	// Copied, because the caller owns the slice it handed us and a document
	// that changed under a run record would defeat the point of addressing it.
	keep := func(name string) {
		r.raws[kind+"/"+name] = append([]byte(nil), data...)
	}

	switch kind {
	case codec.KindDataset:
		ds, err := codec.Loader{}.Dataset(data)
		if err != nil {
			return fmt.Errorf("%s: %w", origin(path), err)
		}
		r.datasets[ds.Name] = ds
		r.paths[kind+"/"+ds.Name] = path
		keep(ds.Name)
	case codec.KindReport:
		rep, err := codec.Loader{}.Report(data)
		if err != nil {
			return fmt.Errorf("%s: %w", origin(path), err)
		}
		r.reports[rep.Name] = rep
		r.paths[kind+"/"+rep.Name] = path
		keep(rep.Name)
	case codec.KindSchedule:
		sc, err := codec.Loader{}.Schedule(data)
		if err != nil {
			return fmt.Errorf("%s: %w", origin(path), err)
		}
		r.schedules[sc.Name] = sc
		r.paths[kind+"/"+sc.Name] = path
		keep(sc.Name)
	case codec.KindDataSource:
		src, err := codec.Loader{}.DataSource(data)
		if err != nil {
			return fmt.Errorf("%s: %w", origin(path), err)
		}
		r.sources[src.Name] = src
		r.paths[kind+"/"+src.Name] = path
		keep(src.Name)
	}
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

// Schedule returns the named schedule.
func (r *Repository) Schedule(_ context.Context, name string) (definition.Schedule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sc, ok := r.schedules[name]
	if !ok {
		return definition.Schedule{}, fmt.Errorf("%w: schedule %q", ErrNotFound, name)
	}
	return sc, nil
}

// Schedules returns every loaded schedule, for a scheduler to arm.
func (r *Repository) Schedules() []definition.Schedule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]definition.Schedule, 0, len(r.schedules))
	for _, s := range r.schedules {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// DataSource returns the named connection definition.
func (r *Repository) DataSource(_ context.Context, name string) (definition.DataSource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	src, ok := r.sources[name]
	if !ok {
		return definition.DataSource{}, fmt.Errorf("%w: datasource %q", ErrNotFound, name)
	}
	return src, nil
}

// Datasets returns everything loaded, for a catalogue to summarise.
func (r *Repository) Datasets() []definition.Dataset {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]definition.Dataset, 0, len(r.datasets))
	for _, d := range r.datasets {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Reports returns everything loaded.
func (r *Repository) Reports() []definition.Report {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]definition.Report, 0, len(r.reports))
	for _, rep := range r.reports {
		out = append(out, rep)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// DataSources returns everything loaded, for a registry to open at startup.
func (r *Repository) DataSources() []definition.DataSource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]definition.DataSource, 0, len(r.sources))
	for _, s := range r.sources {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Counts reports what was loaded, for a startup line an operator can read.
func (r *Repository) Counts() (datasets, reports, schedules, sources int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.datasets), len(r.reports), len(r.schedules), len(r.sources)
}

// origin names a document in an error: the file it came from, or the fact that
// it did not come from one.
func origin(path string) string {
	if path == "" {
		return "published document"
	}
	return path
}
