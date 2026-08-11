package api

import (
	"log/slog"
	"net/http"

	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/token"
)

// Author authenticates somebody who writes reports.
//
// A portal token rather than the admin key. The admin key is a shared secret
// held by a deployment pipeline, and a browser is the one place it must never
// be: anything in a browser is in a devtools console, a screenshot and a
// support ticket.
//
// Real sign-in — password, or the SSO that is a commercial feature — is not
// built. This is the mechanism it will issue against, and it is enforced now
// so the endpoints are never open in the meantime.
type Author struct {
	signer *token.Signer
	// admin lets a deployment pipeline use the same endpoints server to server.
	admin *AdminKey
}

// NewAuthor wires portal authentication.
func NewAuthor(s *token.Signer, admin *AdminKey) *Author {
	return &Author{signer: s, admin: admin}
}

// Principal returns who the request acts as.
//
// The portal token first, because that is the common case; the admin key is
// the pipeline's path and stays available so publishing from CI keeps working.
func (a *Author) Principal(r *http.Request) (principal.Principal, bool) {
	if claims, err := a.signer.Verify(bearer(r), token.Portal); err == nil {
		return claims.Principal(), true
	}
	if a.admin != nil {
		return a.admin.Principal(r)
	}
	return principal.Principal{}, false
}

// Enabled reports whether these endpoints should be mounted at all.
func (a *Author) Enabled() bool { return a.signer != nil }

// Reports serves the portal's read of a report.
//
// A separate path from /v1/embed/reports/{name} rather than one endpoint that
// accepts both audiences: the two have different callers, different failure
// messages and different futures, and collapsing them would make the audience
// check a branch inside a handler rather than the first thing it does.
type PortalReports struct {
	embed *Embed
	auth  *Author
	log   *slog.Logger
}

// NewPortalReports wires the handler.
func NewPortalReports(e *Embed, a *Author, log *slog.Logger) *PortalReports {
	return &PortalReports{embed: e, auth: a, log: log}
}

// ServeHTTP handles POST /v1/reports/{name}.
func (p *PortalReports) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pr, ok := p.auth.Principal(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "Sign in to view this report.")
		return
	}
	if !pr.CanRead() {
		fail(w, http.StatusForbidden, "You do not have access to this project.")
		return
	}
	p.embed.render(w, r, pr, r.PathValue("name"))
}
