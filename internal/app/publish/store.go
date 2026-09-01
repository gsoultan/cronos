package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/gsoultan/cronos/internal/core/principal"
)

// Store keeps definition documents as authors wrote them.
//
// Bytes, not parsed values. The document someone submitted is the artifact a
// run is reproducible against, and re-serialising a parsed definition would
// mean the thing stored is our rendering of their intent rather than their
// intent — comments, ordering and all.
//
// # Every method takes the principal
//
// Not because the store needs to know who is asking, but because it needs to
// know *where*. The organization and project come from the caller's identity
// and never from an argument the caller chose, so there is no shape of request
// that reads another tenant's definitions. A store that took them separately
// would let one be passed while acting in the other, which is the whole of the
// bug class this prevents — see docs/tenancy.md.
type Store interface {
	Put(ctx context.Context, pr principal.Principal, kind, name string, raw []byte) (version string, err error)
	Get(ctx context.Context, pr principal.Principal, kind, name string) ([]byte, error)
	List(ctx context.Context, pr principal.Principal) ([]Entry, error)
	Delete(ctx context.Context, pr principal.Principal, kind, name string) error
}

// Entry is one stored definition, as a listing shows it.
type Entry struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

/*
Version is the content address of a document.

One definition, used by both stores and by the conflict check. It was written
out twice — identically, which is the only reason nothing had gone wrong yet,
and exactly the arrangement where a change to one of them is a silent
disagreement about what version a definition is at.

Truncated to twelve hex characters: a collision every 2^24 documents in one
project, and short enough to appear in a run record somebody reads.
*/
func Version(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])[:12]
}
