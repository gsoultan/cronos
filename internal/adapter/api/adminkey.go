package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gsoultan/cronos/internal/core/principal"
)

// AdminKey authenticates the management API with a shared secret.
//
// A key and not a token: the management API is called by a deployment
// pipeline, not by a browser, and there is nobody to log in. It is checked in
// constant time — a byte-by-byte comparison leaks how much of a guess was
// right, which is enough to finish it.
//
// One key, one project. The file store holds one project's definitions, so
// pretending otherwise here would be a tenancy model that does not exist yet.
// Multi-tenant management arrives with the Postgres store, and its principal
// will come from a real identity rather than from configuration.
type AdminKey struct {
	key     []byte
	org     string
	project string
}

// NewAdminKey returns an authenticator for key.
func NewAdminKey(key []byte, org, project string) *AdminKey {
	return &AdminKey{key: key, org: org, project: project}
}

// Principal returns who the request acts as, and whether it may.
func (a *AdminKey) Principal(r *http.Request) (principal.Principal, bool) {
	presented := bearer(r)
	if presented == "" || len(a.key) == 0 {
		return principal.Principal{}, false
	}
	if subtle.ConstantTimeCompare([]byte(presented), a.key) != 1 {
		return principal.Principal{}, false
	}
	return principal.Principal{
		Subject:   "admin-key",
		OrgID:     a.org,
		ProjectID: a.project,
		// Project admin, not org owner. Publishing definitions is a project
		// operation; managing members and projects is not something a
		// deployment pipeline should be able to do with the same secret.
		ProjectRole: principal.ProjectAdmin,
	}, true
}

// Enabled reports whether management is configured at all.
//
// Absent means the endpoints are not mounted rather than mounted and always
// refusing. A server with no admin key is a read-only server on purpose, and
// an endpoint that exists to say no is an endpoint someone will find.
func (a *AdminKey) Enabled() bool { return len(strings.TrimSpace(string(a.key))) > 0 }
