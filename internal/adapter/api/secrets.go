package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gsoultan/cronos/internal/app/vault"
	"github.com/gsoultan/cronos/internal/core/principal"
)

/*
A project's secrets, set from the portal: a Mapbox token for a map, the
password for a warehouse somebody just connected.

Write-only. There is a list of names, when each was set and by whom, and which
definitions use it — and no request, from anybody, that returns a value. A
secret is set, used, and replaced; somebody who needs to know what it is has
it wherever they copied it from.
*/

// Secrets is what the API may do with the caller's own project's secrets.
type Secrets interface {
	// Storing reports whether this deployment can keep a secret at all.
	Storing() bool
	List(ctx context.Context, pr principal.Principal) ([]vault.Entry, error)
	Set(ctx context.Context, pr principal.Principal, name, value string) error
	Delete(ctx context.Context, pr principal.Principal, name string) error
}

// maxSecretBody is well above vault.MaxValueBytes: JSON may spend six bytes on
// one, and a value refused for its length should say so rather than "too large".
const maxSecretBody = 128 << 10

// SecretsAPI serves /v1/secrets and /v1/secrets/{name}.
type SecretsAPI struct {
	secrets Secrets
	auth    Principals
	log     *slog.Logger
}

// NewSecretsAPI wires the handler.
func NewSecretsAPI(s Secrets, a Principals, log *slog.Logger) *SecretsAPI {
	return &SecretsAPI{secrets: s, auth: a, log: log}
}

// ServeHTTP handles GET on the list, and PUT and DELETE on one name.
func (h *SecretsAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := h.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Not authorised.")
		return
	}
	name := r.PathValue("name")
	switch {
	case r.Method == http.MethodGet && name == "":
		h.list(w, r, pr)
	case r.Method == http.MethodPut && name != "":
		h.set(w, r, pr, name)
	case r.Method == http.MethodDelete && name != "":
		h.delete(w, r, pr, name)
	default:
		fail(w, http.StatusMethodNotAllowed, "Not a method this endpoint takes.")
	}
}

func (h *SecretsAPI) list(w http.ResponseWriter, r *http.Request, pr principal.Principal) {
	entries, err := h.secrets.List(r.Context(), pr)
	if err != nil {
		h.refuse(w, err)
		return
	}
	send(w, http.StatusOK, map[string]any{"store": h.secrets.Storing(), "secrets": entries})
}

func (h *SecretsAPI) set(w http.ResponseWriter, r *http.Request, pr principal.Principal, name string) {
	var in struct {
		Value string `json:"value"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSecretBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "Send the value as {\"value\": \"…\"}.")
		return
	}
	if err := h.secrets.Set(r.Context(), pr, name, in.Value); err != nil {
		h.audited(r.Context(), pr, ActionSecretSet, name, err)
		h.refuse(w, err)
		return
	}
	// The name and never the value, here and in the audit entry: the log is
	// the most widely read thing a deployment has.
	h.log.Info("secret set", "project", pr.OrgID+"/"+pr.ProjectID, "secret", name, "by", pr.Subject)
	audit(r.Context(), h.log, pr, ActionSecretSet, name, Allowed, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *SecretsAPI) delete(w http.ResponseWriter, r *http.Request, pr principal.Principal, name string) {
	if err := h.secrets.Delete(r.Context(), pr, name); err != nil {
		h.audited(r.Context(), pr, ActionSecretDelete, name, err)
		h.refuse(w, err)
		return
	}
	h.log.Info("secret deleted", "project", pr.OrgID+"/"+pr.ProjectID, "secret", name, "by", pr.Subject)
	audit(r.Context(), h.log, pr, ActionSecretDelete, name, Allowed, nil)
	w.WriteHeader(http.StatusNoContent)
}

// audited records a refusal worth an auditor's attention: somebody reaching
// for a project's secrets without the role to.
func (h *SecretsAPI) audited(ctx context.Context, pr principal.Principal, action, name string, err error) {
	if errors.Is(err, vault.ErrForbidden) {
		audit(ctx, h.log, pr, action, name, Refused, map[string]any{"reason": "not an editor here"})
	}
}

// refuse maps a vault failure to a status and a sentence somebody can act on.
func (h *SecretsAPI) refuse(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vault.ErrForbidden):
		fail(w, http.StatusForbidden, "Only an editor of this project may manage its secrets.")
	case errors.Is(err, vault.ErrUnavailable):
		fail(w, http.StatusServiceUnavailable, "This deployment has no CRONOS_SECRETS_KEY, so it "+
			"has nowhere safe to keep a secret. Set one and restart — see docs/deploying.md.")
	case errors.Is(err, vault.ErrInvalid):
		fail(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, vault.ErrNotFound):
		fail(w, http.StatusNotFound, "No such secret is stored here.")
	case errors.Is(err, vault.ErrFull):
		fail(w, http.StatusConflict, err.Error())
	default:
		h.log.Error("secrets failed", "err", err)
		fail(w, http.StatusInternalServerError, "Could not reach the secrets store.")
	}
}
