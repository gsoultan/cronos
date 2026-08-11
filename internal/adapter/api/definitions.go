package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/app/publish"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// maxDefinition caps a submitted document. A dataset is a few kilobytes of
// YAML; a megabyte of it is a mistake or an attempt to make the server hold it.
const maxDefinition = 256 << 10

// Definitions serves the management API.
// Principals is whatever decides who a management request acts as: the shared
// key a pipeline holds, or a portal token an author signed in with.
type Principals interface {
	Principal(r *http.Request) (principal.Principal, bool)
}

// Loaded is whatever the server is actually running, for the definitions no
// store has a copy of.
type Loaded interface {
	Raw(kind, name string) ([]byte, bool)
}

type Definitions struct {
	svc   *publish.Service
	store publish.Store
	auth  Principals
	log   *slog.Logger
	// loaded is what the process booted with, and org and project are the one
	// tenant it holds. A directory is not multi-tenant, so serving it to any
	// principal that asked would be a cross-project read through the one path
	// that does not go via a store — which is where such a hole would live.
	loaded  Loaded
	org     string
	project string
}

// NewDefinitions wires the handler.
func NewDefinitions(s *publish.Service, st publish.Store, a Principals, log *slog.Logger) *Definitions {
	return &Definitions{svc: s, store: st, auth: a, log: log}
}

// WithLoaded lets a read fall back to what the server booted with.
//
// A deployment whose definitions came from a directory and whose store is a
// database has both: the store holds what anybody published, and the directory
// holds everything else. Without the fallback, a report that plainly renders
// answers 404 when asked what it says, which is a contradiction an author
// cannot act on.
func (d *Definitions) WithLoaded(l Loaded, org, project string) *Definitions {
	d.loaded, d.org, d.project = l, org, project
	return d
}

// ServeHTTP handles /v1/definitions and /v1/definitions/{kind}/{name}.
func (d *Definitions) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := d.auth.Principal(r)
	if !ok {
		// The same sentence for a missing key and a wrong one.
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}

	kind, name := r.PathValue("kind"), r.PathValue("name")
	switch {
	case r.Method == http.MethodPost && kind == "":
		d.publish(w, r, pr)
	case r.Method == http.MethodGet && kind == "":
		d.list(w, r, pr)
	case r.Method == http.MethodGet:
		d.get(w, r, pr, kind, name)
	case r.Method == http.MethodDelete:
		d.delete(w, r, pr, kind, name)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

func (d *Definitions) publish(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDefinition))
	if err != nil {
		fail(w, http.StatusRequestEntityTooLarge, "That document is too large.")
		return
	}

	result, err := d.svc.Publish(r.Context(), raw, pr)
	if err != nil {
		d.refuse(w, err)
		return
	}
	d.log.Info("published", "kind", result.Kind, "name", result.Name, "version", result.Version)
	send(w, http.StatusOK, result)
}

func (d *Definitions) list(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	entries, err := d.store.List(r.Context(), pr)
	if err != nil {
		d.log.Error("listing definitions failed", "err", err)
		fail(w, http.StatusInternalServerError, "Could not read the definitions.")
		return
	}
	if entries == nil {
		// An empty array, never null: a client iterating the response should
		// not have to know which one an empty repository produces.
		entries = []publish.Entry{}
	}
	send(w, http.StatusOK, map[string]any{"definitions": entries})
}

// get returns the document exactly as it was submitted.
//
// The bytes, not a re-serialisation: comments and ordering are the author's,
// and a management API that hands back its own rendering makes every round
// trip a diff.
func (d *Definitions) get(w http.ResponseWriter, r *http.Request, pr principal.Principal, kind, name string) {
	raw, err := d.store.Get(r.Context(), pr, canonicalKind(kind), name)
	if err != nil {
		// The store first, so a published edit wins over the file it was read
		// from — the fallback answers for what nobody has changed yet.
		fallback, ok := d.fromLoaded(pr, canonicalKind(kind), name)
		if !ok {
			fail(w, http.StatusNotFound, "No such definition.")
			return
		}
		raw = fallback
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}

// fromLoaded answers from the running view, which is scoped to the one
// project this process serves — so a principal from another may not read it.
func (d *Definitions) fromLoaded(pr principal.Principal, kind, name string) ([]byte, bool) {
	if d.loaded == nil || pr.OrgID != d.org || pr.ProjectID != d.project {
		return nil, false
	}
	return d.loaded.Raw(kind, name)
}

// delete removes a definition, through the service rather than the store.
//
// The store checks the tenant and nothing else. Going straight to it meant a
// viewer's token could remove whatever it named, and that whatever it removed
// might be the dataset a report reads.
func (d *Definitions) delete(w http.ResponseWriter, r *http.Request, pr principal.Principal, kind, name string) {
	if err := d.svc.Delete(r.Context(), pr, canonicalKind(kind), name); err != nil {
		d.refuse(w, err)
		return
	}
	d.log.Info("deleted", "kind", kind, "name", name, "by", pr.Subject)
	w.WriteHeader(http.StatusNoContent)
}

// refuse maps a publish failure to a status.
//
// Validation messages are returned in full, and that is the point of the
// endpoint: an author publishing from a pipeline gets the sentence that says
// which field is wrong, not a status code and a shrug.
func (d *Definitions) refuse(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, publish.ErrForbidden):
		fail(w, http.StatusForbidden, "Not permitted to change definitions.")
	case errors.Is(err, publish.ErrUnsupported):
		fail(w, http.StatusBadRequest, err.Error())
	// Conflict, not a bad request: nothing about what was asked is malformed,
	// and the sentence names what would break so somebody can go and fix it.
	case errors.Is(err, publish.ErrInUse):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, publish.ErrNotFound):
		fail(w, http.StatusNotFound, err.Error())
	case
		errors.Is(err, codec.ErrDecode),
		errors.Is(err, definition.ErrInvalid),
		errors.Is(err, query.ErrBadTemplate):
		fail(w, http.StatusUnprocessableEntity, err.Error())
	default:
		d.log.Error("publish failed", "err", err)
		fail(w, http.StatusInternalServerError, "Could not store the definition.")
	}
}

// canonicalKind maps the plural in a URL to the kind in a document.
//
// URLs are lowercase and plural because that is what a REST client expects;
// documents say Dataset because that is what the format says. Mapping between
// them here keeps both idiomatic instead of making one of them ugly.
func canonicalKind(urlKind string) string {
	switch strings.ToLower(urlKind) {
	case "datasets", "dataset":
		return codec.KindDataset
	case "reports", "report":
		return codec.KindReport
	}
	return urlKind
}
