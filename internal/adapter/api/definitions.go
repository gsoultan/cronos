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
type Definitions struct {
	svc   *publish.Service
	store publish.Store
	auth  *AdminKey
	log   *slog.Logger
}

// NewDefinitions wires the handler.
func NewDefinitions(s *publish.Service, st publish.Store, a *AdminKey, log *slog.Logger) *Definitions {
	return &Definitions{svc: s, store: st, auth: a, log: log}
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
		d.list(w, r)
	case r.Method == http.MethodGet:
		d.get(w, r, kind, name)
	case r.Method == http.MethodDelete:
		d.delete(w, r, kind, name)
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

func (d *Definitions) list(w http.ResponseWriter, r *http.Request) {
	entries, err := d.store.List(r.Context())
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
func (d *Definitions) get(w http.ResponseWriter, r *http.Request, kind, name string) {
	raw, err := d.store.Get(r.Context(), canonicalKind(kind), name)
	if err != nil {
		fail(w, http.StatusNotFound, "No such definition.")
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}

func (d *Definitions) delete(w http.ResponseWriter, r *http.Request, kind, name string) {
	if err := d.store.Delete(r.Context(), canonicalKind(kind), name); err != nil {
		fail(w, http.StatusNotFound, "No such definition.")
		return
	}
	d.log.Info("deleted", "kind", kind, "name", name)
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
	case errors.Is(err, publish.ErrNotFound),
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
