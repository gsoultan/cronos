package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// safeName is the only shape a stored definition's name may take.
//
// Names reach the filesystem here, and this is the check that makes traversal
// impossible rather than filtered. definition.Validate enforces the same shape
// on the way in; repeating it is cheap, and the consequence of the two
// disagreeing is a file written outside the repository.
var safeName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Writer stores definitions as files under a directory.
//
// Files, because that is what the format is for: a definitions directory is a
// git repository, and a publish is a commit somebody can review. A hosted
// deployment wants Postgres and the same ports.
type Writer struct {
	dir string
	// org and project are the single tenant this directory holds.
	//
	// Checked rather than ignored. A store that quietly served every tenant
	// the same files would be indistinguishable from a working multi-tenant
	// one right up until the first customer saw another's report.
	org     string
	project string
	// mu serialises writes. Two publishes of the same definition would
	// otherwise interleave a rename with a read, and the loser leaves a
	// half-written file where a definition used to be.
	mu sync.Mutex
	// repo is refreshed after every write so a running server serves what was
	// just published rather than what it read at startup.
	repo *Repository
}

// NewWriter returns a Writer over dir, sharing repo's view, holding one
// project's definitions.
func NewWriter(dir, org, project string, repo *Repository) *Writer {
	return &Writer{dir: dir, org: org, project: project, repo: repo}
}

// tenant refuses a principal acting somewhere this directory does not hold.
//
// A directory is one project. Serving it to whoever asks would work perfectly
// until the day a second project existed.
func (w *Writer) tenant(pr principal.Principal) error {
	if pr.OrgID != w.org || pr.ProjectID != w.project {
		return fmt.Errorf("%w: this repository holds %s/%s, not %s/%s",
			publish.ErrNotFound, w.org, w.project, pr.OrgID, pr.ProjectID)
	}
	return nil
}

// Version is the content address of a document.
//
// Truncated to twelve hex characters, which is a collision every 2^24
// documents in one project and short enough to appear in a run record someone
// reads. The full digest is the file's name in the version directory.
func Version(raw []byte) string { return publish.Version(raw) }

// Put writes a definition and keeps the previous content addressable.
func (w *Writer) Put(_ context.Context, pr principal.Principal, kind, name string, raw []byte) (string, error) {
	if err := w.tenant(pr); err != nil {
		return "", err
	}
	if !safeName.MatchString(name) {
		return "", fmt.Errorf("file: %q is not a definition name", name)
	}
	path, err := w.pathFor(kind, name)
	if err != nil {
		return "", err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	// The version copy is written first. If the process dies between the two,
	// the history has an entry nothing points at — which is inert. The other
	// order loses the ability to reproduce what is now live.
	if err := w.keep(kind, name, raw); err != nil {
		return "", err
	}
	if err := writeAtomic(path, raw); err != nil {
		return "", err
	}
	return Version(raw), w.reload()
}

// keep files the document under its full digest, so a run record naming a
// version can be replayed against the exact bytes that produced it.
func (w *Writer) keep(kind, name string, raw []byte) error {
	sum := sha256.Sum256(raw)
	dir := filepath.Join(w.dir, ".versions", kind, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, hex.EncodeToString(sum[:])+".yaml"), raw)
}

// Get returns the stored document.
func (w *Writer) Get(_ context.Context, pr principal.Principal, kind, name string) ([]byte, error) {
	if err := w.tenant(pr); err != nil {
		return nil, err
	}
	path, err := w.pathFor(kind, name)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s %q", publish.ErrNotFound, kind, name)
	}
	return raw, nil
}

// Delete removes a definition. The version history is left alone: a run that
// used it must still be reproducible, and deleting a definition is not a claim
// that it never existed.
func (w *Writer) Delete(_ context.Context, pr principal.Principal, kind, name string) error {
	if err := w.tenant(pr); err != nil {
		return err
	}
	path, err := w.pathFor(kind, name)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("%w: %s %q", publish.ErrNotFound, kind, name)
	}
	return w.reload()
}

// List returns what the repository loaded, with each document's version.
//
// The repository rather than the directory layout: Load walks recursively and
// an author may have organised their files by team, so scanning two fixed
// folders would report an empty repository that is plainly not empty.
func (w *Writer) List(ctx context.Context, pr principal.Principal) ([]publish.Entry, error) {
	if err := w.tenant(pr); err != nil {
		return nil, err
	}
	var out []publish.Entry
	for _, kind := range []string{
		codec.KindDataSource, codec.KindDataset, codec.KindReport, codec.KindSchedule,
	} {
		for _, name := range w.repo.Names(kind) {
			raw, err := w.Get(ctx, pr, kind, name)
			if err != nil {
				continue
			}
			out = append(out, publish.Entry{Kind: kind, Name: name, Version: Version(raw)})
		}
	}
	return out, nil
}

// pathFor is where a definition lives: where it was read from if it is known,
// and the canonical folder if it is new.
func (w *Writer) pathFor(kind, name string) (string, error) {
	if !safeName.MatchString(name) {
		// Names reach the filesystem here, so this is the check that makes
		// traversal impossible rather than filtered.
		return "", fmt.Errorf("file: %q is not a definition name", name)
	}
	if path, ok := w.repo.Path(kind, name); ok {
		return path, nil
	}
	switch kind {
	/*
	   A datasource is a definition here like any other.

	   It was not, and the asymmetry was invisible because Load indexes the kind
	   and records its path: editing a datasource that already existed on disk
	   worked, and only creating one fell through to the refusal below. So a
	   file-backed deployment could change a connection and not add one, which is
	   the wrong way round — adding is what somebody does on their first
	   afternoon.

	   The secret in a `dsn` is a reference to be resolved, not the credential
	   itself; a definitions directory is a git repository and writing a password
	   into one is the thing secret resolution exists to avoid.
	*/
	case codec.KindDataSource:
		return filepath.Join(w.dir, "datasources", name+".yaml"), nil
	case codec.KindDataset:
		return filepath.Join(w.dir, "datasets", name+".yaml"), nil
	case codec.KindReport:
		return filepath.Join(w.dir, "reports", name+".yaml"), nil
	case codec.KindSchedule:
		return filepath.Join(w.dir, "schedules", name+".yaml"), nil
	}
	return "", fmt.Errorf("%w: %s", publish.ErrUnsupported, kind)
}

// reload re-reads the directory into the repository the server is serving.
func (w *Writer) reload() error {
	fresh, err := Load(w.dir)
	if err != nil {
		return err
	}
	w.repo.replace(fresh)
	return nil
}

// writeAtomic writes through a temporary file in the same directory.
//
// A definition read while it is being overwritten is a truncated document, and
// the reader's error would be a parse failure pointing at a line the author
// never wrote. Rename within a directory is atomic; write-in-place is not.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
