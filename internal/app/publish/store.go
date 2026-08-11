package publish

import "context"

// Store keeps definition documents as authors wrote them.
//
// Bytes, not parsed values. The document someone submitted is the artifact a
// run is reproducible against, and re-serialising a parsed definition would
// mean the thing stored is our rendering of their intent rather than their
// intent — comments, ordering and all.
type Store interface {
	Put(ctx context.Context, kind, name string, raw []byte) (version string, err error)
	Get(ctx context.Context, kind, name string) ([]byte, error)
	List(ctx context.Context) ([]Entry, error)
	Delete(ctx context.Context, kind, name string) error
}

// Entry is one stored definition, as a listing shows it.
type Entry struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version"`
}
