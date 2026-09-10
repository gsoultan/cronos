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
	// confining resolves the row scope an account reads through. Read on the
	// same cached path as standing, because both answer questions about the
	// account rather than about the token.
	confining Confining
	mu        sync.Mutex
	active    map[string]moment

	signer *token.Signer
	// admin lets a deployment pipeline use the same endpoints server to server.
	admin *AdminKey
}

// NewAuthor wires portal authentication.
func NewAuthor(s *token.Signer, admin *AdminKey) *Author {
	return &Author{signer: s, admin: admin}
}

/*
Confining resolves the row scope an account reads through.

Declared here because this is the package that needs it, and optional because a
file-backed deployment has no store to ask — where it is absent nobody is
confined, which is exactly the behaviour before the feature existed.
*/
type Confining interface {
	Confinement(ctx context.Context, org, project, userID string) (map[string]string, error)
}

// WithConfinement makes a viewer read through the scope their administrator
// set for them, or their group's.
func (a *Author) WithConfinement(c Confining) *Author { a.confining = c; return a }

// Standing is whether the person a token names is an account here, and
// whether it may still act.
type Standing interface {
	/*
	   Active reports whether this subject may act, and from when.

	   `since` is the moment their sessions became valid. A token minted before
	   it is refused however good its signature — which is how "sign out
	   everywhere" works without storing sessions: there is no list to walk,
	   only a line drawn in time that every token is checked against.

	   Zero means no line has been drawn, which is every account until somebody
	   presses the button.
	*/
	Active(ctx context.Context, id string) (known, active bool, since time.Time)
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
	// since is when this account's sessions became valid, held so the cached
	// answer can still refuse a token minted before it.
	since time.Time
	/*
	   confined is the row scope this account reads through, from its own
	   scope or its groups'.

	   Resolved here rather than minted into the token, and that is the whole
	   reason it is in this struct. There are six places a portal token is
	   issued, and a confinement threaded through all of them is one somebody
	   eventually forgets to thread — a fail-open that shows a viewer every
	   region. Resolving it where the principal is built is one place, and it
	   rides the cache this check already keeps.

	   The cost is that a scope change takes effect on the next lookup rather
	   than instantly, which is the same five seconds a disabled account takes.
	*/
	confined map[string]string
	// broken records that the confinement could not be read at all. A person
	// whose scope is unreadable must not be treated as unconfined.
	broken bool
}

// stands reports whether this token may still act, from a short cache.
//
// Takes the claims rather than the subject: two of the three questions are
// about the account, and the third — whether this token predates the last "sign
// out everywhere" — is about the token.
func (a *Author) stands(ctx context.Context, claims token.Claims) bool {
	subject := claims.Subject
	if a.standing == nil || subject == "" {
		return true
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	const remember = 5 * time.Second
	now := time.Now()
	if cached, ok := a.active[subject]; ok && now.Sub(cached.at) < remember {
		return cached.ok && minted(claims, cached.since)
	}

	// Sweeping here rather than on a timer: the map is only read on this path,
	// and a deployment with a thousand people has a thousand entries, not a
	// goroutine.
	for id, when := range a.active {
		if now.Sub(when.at) > remember {
			delete(a.active, id)
		}
	}

	known, active, since := a.standing.Active(ctx, subject)
	// A subject that is not an account here is a machine credential — a
	// pipeline's token, or one baked into a portal build — and those are
	// governed by their signature and expiry, which is all there has ever been
	// to govern them by. Refusing them was locking out every deployment that
	// mints its own.
	ok := !known || active
	if !known {
		/*
		   Not an account here, so no line applies.

		   A machine credential — a pipeline's token, one baked into a portal
		   build — is governed by its signature and expiry, which is all there
		   has ever been to govern it by. Cleared here rather than trusted to
		   arrive zero, because "somebody's sign-out ended every CI token in
		   the deployment" is the failure this check already had once, and a
		   store that answered a stale timestamp for a missing row would bring
		   it back.
		*/
		since = time.Time{}
	}
	// Only for a role the answer could apply to. An admin's confinement is a
	// query whose result is discarded, and on the sign-in path that is a round
	// trip per request for nothing.
	var confined map[string]string
	var broken bool
	if claims.Principal().Confinable() {
		confined, broken = a.confinement(ctx, claims)
	}
	a.active[subject] = moment{ok: ok, at: now, since: since, confined: confined, broken: broken}
	return ok && minted(claims, since)
}

/*
confinement reads the scope this account is held to, and says so if it cannot.

broken rather than an error return, because the caller's question is "may this
token act" and an unreadable confinement is not a no to that — it is a yes with
no scope, which is the one answer that must not be given. Principal refuses
instead, so a store that cannot answer costs a viewer their session rather than
costing them their confinement.
*/
func (a *Author) confinement(ctx context.Context, claims token.Claims) (map[string]string, bool) {
	if a.confining == nil {
		return nil, false
	}
	// The project the token names, so a membership row belonging to another
	// cannot confine — or fail to confine — somebody here.
	scope, err := a.confining.Confinement(ctx, claims.Org, claims.Project, claims.Subject)
	if err != nil {
		return nil, true
	}
	return scope, false
}

// confinedScope returns the cached scope for a subject, and whether reading it
// failed. Called after stands, so the entry is present.
func (a *Author) confinedScope(subject string) (map[string]string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if m, ok := a.active[subject]; ok {
		return m.confined, m.broken
	}
	return nil, false
}

/*
minted reports whether this token was issued after the line was drawn.

This is the whole of session revocation, and the reason it is affordable:
nothing is stored per session, so nothing has to be walked. One timestamp per
account invalidates every token issued before it — including the ones on
machines nobody has access to any more, which is the only reason the button
exists.

What it cannot do is end one session and leave another, because a stateless
token carries nothing to tell them apart. That is a real limit and the interface
says so rather than offering a list of devices it cannot produce.
*/
func minted(claims token.Claims, since time.Time) bool {
	if since.IsZero() {
		/*
		   No line drawn, which is every account until somebody presses the
		   button.

		   Redundant, strictly: a zero time's Unix() is around -62135596800, so
		   the comparison below already lets everything through. Kept because
		   the rule is worth stating where it can be read, and because a
		   representation that ever stopped having that property would
		   otherwise turn "nobody has pressed the button" into "nothing is
		   valid" silently.
		*/
		return true
	}
	/*
	   Whole seconds, and `>=` rather than `>`.

	   `iat` has second granularity, so a token minted in the same second as the
	   cut-off must survive it — otherwise pressing the button ends the session
	   of the person pressing it, from the very request that pressed it, and
	   they are bounced to the sign-in page for doing the right thing.
	*/
	return claims.IssuedAt >= since.Unix()
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
		if !a.stands(r.Context(), claims) {
			return principal.Principal{}, false
		}

		pr := claims.Principal()
		/*
		   The scope an administrator confined this person to.

		   Applied after the claims rather than inside them: a portal token
		   carries no scope of its own, and if one ever did, a confinement set
		   in the deployment must win over anything a token asserts about
		   itself.

		   A confinement that could not be read refuses the request. The
		   alternative is serving a viewer every region because a query failed,
		   which is the failure this whole feature exists to prevent — and an
		   unreadable scope is a broken deployment, not an unconfined person.
		*/
		if pr.Confinable() {
			scope, broken := a.confinedScope(claims.Subject)
			if broken {
				/*
				   We cannot tell whether this person is confined, and they are
				   in the one role that could be. Refused rather than served
				   unconfined.

				   Asked only of a viewer, and that is what keeps this from
				   being an outage. An editor or an admin is exempt from row
				   scope whatever the answer, so a store that has gone away
				   still serves them — which is the property
				   scripts/live-failover.sh holds, and the reason somebody can
				   still read a report while they fix the database.
				*/
				return principal.Principal{}, false
			}
			if len(scope) > 0 {
				pr.Scope = scope
			}
		}
		return pr, true
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
	gate  gate
	embed *Embed
	auth  *Author
	log   *slog.Logger
}

// NewPortalReports wires the handler.
func NewPortalReports(e *Embed, a *Author, log *slog.Logger) *PortalReports {
	return &PortalReports{embed: e, auth: a, log: log}
}

// WithGrants restricts reports somebody has granted to named people or groups.
// Absent, every report in the project opens for anybody who may read it, which
// is the behaviour before grants existed.
func (p *PortalReports) WithGrants(g Granting) *PortalReports {
	p.gate = gate{grants: g, log: p.log}
	return p
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

	name := r.PathValue("name")
	allowed, known := p.gate.may(r.Context(), pr, name)
	if !known {
		// The grants could not be read. Refusing costs somebody a report;
		// guessing would open every restricted one in the deployment at the
		// moment the database is least well.
		fail(w, http.StatusServiceUnavailable, "Could not check who may open this report.")
		return
	}
	if !allowed {
		/*
		   The same answer a report that does not exist would give.

		   A 403 here tells somebody a report is there and that they are not on
		   its list, which is a fact about the project nobody granted them —
		   and on a list of report names it is an enumeration oracle.
		*/
		audit(r.Context(), p.log, pr, ActionRead, name, Refused,
			map[string]any{"reason": "not granted"})
		fail(w, http.StatusNotFound, "No such report.")
		return
	}
	p.embed.render(w, r, pr, name)
}

// ForgetStanding drops the cached answers, so a test does not wait five seconds
// for a change to take effect.
func ForgetStanding(a *Author) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.active = map[string]moment{}
}
