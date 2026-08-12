package api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

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
	// standing answers whether the person a token names still works here.
	// Absent, a token is governed by its expiry alone — which is correct for a
	// deployment with no user store, where nobody can be disabled.
	standing Standing
	mu       sync.Mutex
	active   map[string]moment

	signer *token.Signer
	// admin lets a deployment pipeline use the same endpoints server to server.
	admin *AdminKey
}

// NewAuthor wires portal authentication.
func NewAuthor(s *token.Signer, admin *AdminKey) *Author {
	return &Author{signer: s, admin: admin}
}

// Standing is whether the person a token names is an account here, and
// whether it may still act.
type Standing interface {
	Active(ctx context.Context, id string) (known, active bool)
}

/*
WithStanding checks a portal token against the account it names.

A token is signed and lives eight hours. Without this, disabling somebody who
holds one takes away nothing until it expires — and "revoked" that means
"revoked by this evening" is not what anybody means when they say it,
particularly on the afternoon somebody is walked out of a building.

The answer is cached for a few seconds. A database round trip on every request
to ask whether a session is still a session is a cost paid by everybody so that
a rare event is instant, and a few seconds is close enough to instant for the
event this exists for.
*/
func (a *Author) WithStanding(s Standing) *Author {
	a.standing = s
	a.active = map[string]moment{}
	return a
}

type moment struct {
	ok bool
	at time.Time
}

// stands reports whether the subject may still act, from a short cache.
func (a *Author) stands(ctx context.Context, subject string) bool {
	if a.standing == nil || subject == "" {
		return true
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	const remember = 5 * time.Second
	now := time.Now()
	if cached, ok := a.active[subject]; ok && now.Sub(cached.at) < remember {
		return cached.ok
	}

	// Sweeping here rather than on a timer: the map is only read on this path,
	// and a deployment with a thousand people has a thousand entries, not a
	// goroutine.
	for id, when := range a.active {
		if now.Sub(when.at) > remember {
			delete(a.active, id)
		}
	}

	known, active := a.standing.Active(ctx, subject)
	// A subject that is not an account here is a machine credential — a
	// pipeline's token, or one baked into a portal build — and those are
	// governed by their signature and expiry, which is all there has ever been
	// to govern them by. Refusing them was locking out every deployment that
	// mints its own.
	ok := !known || active
	a.active[subject] = moment{ok: ok, at: now}
	return ok
}

// Principal returns who the request acts as.
//
// The portal token first, because that is the common case; the admin key is
// the pipeline's path and stays available so publishing from CI keeps working.
func (a *Author) Principal(r *http.Request) (principal.Principal, bool) {
	if claims, err := a.signer.Verify(bearer(r), token.Portal); err == nil {
		// Signed, unexpired, and still somebody who works here. The first two
		// are what the signature proves; the third is the one that changes
		// after the token was issued.
		if !a.stands(r.Context(), claims.Subject) {
			return principal.Principal{}, false
		}
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
