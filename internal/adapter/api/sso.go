package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/core/identity"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/extension"
	"github.com/gsoultan/cronos/internal/platform/token"
)

/*
SSO is the two ends of a browser sign-in through somebody else's directory.

The person leaves for their identity provider and comes back, so it is two
requests with nothing in common but a cookie: Start sends them, Complete
receives them. What happens in between belongs to the provider, and this
handler mentions no protocol — a SAML implementation would use the same two
routes.

Mounted only where a provider registered. A sign-in button that leads to an
error is worse than no button, and an endpoint that exists to say no is one
somebody spends an afternoon probing.
*/
type SSO struct {
	flow   extension.SignInFlow
	signer *token.Signer
	users  Directory
	// auth reads the session a sign-out is ending, to find its hint.
	auth Principals
	log  *slog.Logger

	// behindProxy is the deployment's own statement about its front. See secure().
	behindProxy bool

	// Where the deployment's people belong when the provider does not say.
	org, project, role string

	mu     sync.Mutex
	states map[string]extension.State
	/*
	   hints holds each session's identity token, for a later sign-out.

	   In memory, keyed by the account it belongs to, and nowhere else. A
	   provider's identity token is a credential at that provider: in a
	   database it is a credential in a backup, and in the browser it is one in
	   a page. A restart loses them, and a sign-out then goes without a hint —
	   which some providers refuse, and which is the honest outcome, because
	   the local session has already ended by then.

	   Dropped when the session it belongs to expires, because a hint outlives
	   its usefulness the moment the session does, and what is left is a
	   credential held for a sign-out that can no longer happen.
	*/
	hints map[string]hint
	now   func() time.Time
}

// Directory is how a person the provider vouched for becomes somebody here.
type Directory interface {
	Upsert(ctx context.Context, u identity.User) (identity.User, error)
}

// NewSSO wires the handler.
func NewSSO(f extension.SignInFlow, s *token.Signer, d Directory,
	auth Principals, log *slog.Logger) *SSO {
	return &SSO{
		flow: f, signer: s, users: d, auth: auth, log: log,
		states: map[string]extension.State{},
		hints:  map[string]hint{},
		now:    time.Now,
	}
}

// In names where people land when the provider does not say.
func (h *SSO) In(org, project, role string) *SSO {
	h.org, h.project, h.role = org, project, role
	return h
}

// BehindProxy says something in front terminates TLS, so the state cookie is
// Secure on a request that reached this process over plaintext.
func (h *SSO) BehindProxy(trusted bool) *SSO {
	h.behindProxy = trusted
	return h
}

/*
secure decides the Secure attribute on the state cookie.

r.TLS alone answers the wrong question. It says how the connection to *this
process* was made, and what the attribute has to describe is how the browser
reached the deployment — which, behind a terminating proxy, is not the same
thing. Getting it from the operator is the only way to know: the proxy's own
X-Forwarded-Proto is a header, and a header is something the caller can set.
*/
func (h *SSO) secure(r *http.Request) bool {
	return r.TLS != nil || h.behindProxy
}

// stateCookie carries the id of a sign-in in progress.
//
// HttpOnly, because nothing in the page needs to read it and a script that can
// is a script that can complete somebody else's sign-in. SameSite=Lax rather
// than Strict: the browser arrives back from the identity provider on a
// top-level navigation, and Strict withholds the cookie on exactly that.
const stateCookie = "cronos_sso"

// ServeHTTP handles /v1/auth/sso/start and /v1/auth/sso/callback.
func (h *SSO) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/start"):
		h.start(w, r)
	case strings.HasSuffix(r.URL.Path, "/callback"):
		h.complete(w, r)
	case strings.HasSuffix(r.URL.Path, "/logout"):
		h.logout(w, r)
	default:
		fail(w, http.StatusNotFound, "No such endpoint.")
	}
}

func (h *SSO) start(w http.ResponseWriter, r *http.Request) {
	// Where to land afterwards, and only somewhere in this portal. An open
	// redirect on a sign-in route is how somebody is sent to a page that looks
	// like this one and asks for their password again.
	returning := safeReturn(r.URL.Query().Get("returning"))

	redirect, state, err := h.flow.Start(r.Context(), returning)
	if err != nil {
		h.log.Error("could not start a sign-in", "provider", h.flow.Name(), "err", err)
		fail(w, http.StatusBadGateway, "Could not reach the identity provider.")
		return
	}

	h.remember(state)
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state.ID, Path: "/",
		HttpOnly: true, Secure: h.secure(r), SameSite: http.SameSiteLaxMode,
		Expires: state.Expires,
	})
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (h *SSO) complete(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(stateCookie)
	if err != nil {
		// No cookie is a callback that did not start here: a stale bookmark, a
		// replayed link, or somebody else's sign-in being finished by this
		// browser. All three end the same way.
		h.refuse(w, r, "this sign-in did not start here")
		return
	}
	state, ok := h.recall(cookie.Value)
	if !ok {
		h.refuse(w, r, "this sign-in expired")
		return
	}

	who, err := h.flow.Complete(r.Context(), r, state)
	if err != nil {
		/*
		   Which system to go and look at.

		   This said "the identity provider refused this sign-in" for every way
		   Complete can fail, and only one of them is that. The others are
		   cronos refusing a token, a deployment configured against a different
		   client, or two machines disagreeing about the time. Naming the
		   provider for those costs somebody an afternoon in the wrong admin
		   console — it cost one here, when a host clock jumped and the first
		   thing restarted was Keycloak.
		*/
		reason, why := "the identity provider refused this sign-in", "provider"
		switch {
		case errors.Is(err, extension.ErrClockSkew):
			reason = "this server and the identity provider disagree about the time"
			why = "clock"
		case errors.Is(err, extension.ErrNotAcceptable):
			reason = "that account is not one this deployment admits"
			why = "not-admitted"
		case errors.Is(err, extension.ErrProviderRefused):
			// The default, and now the one case that has earned it.
		default:
			reason = "this sign-in could not be verified"
			why = "unverified"
		}

		h.log.Info("sign-in refused", "provider", h.flow.Name(), "reason", why, "err", err)
		audit(r.Context(), h.log, principal.Principal{Subject: "sso"},
			ActionSignIn, "sso", Refused,
			map[string]any{"provider": h.flow.Name(), "reason": why})
		h.refuse(w, r, reason)
		return
	}

	user, err := h.adopt(r.Context(), who)
	if err != nil {
		h.log.Error("could not place a person from the provider", "err", err)
		h.refuse(w, r, "you were signed in, but this project could not accept you")
		return
	}

	issued, err := h.signer.Mint(token.Claims{
		Audience: token.Portal, Role: user.Role,
		Org: user.Org, Project: user.Project, Subject: user.ID,
		// Set by the directory, not by the provider. A deployment
		// administrator who signs in through their company's SSO is the same
		// account as one who signs in with a password, and a claim that
		// depended on which door they used would be a surprise nobody enjoys.
		Platform: user.Platform,
	}, SessionLifetime)
	if err != nil {
		h.log.Error("could not mint a session", "err", err)
		h.refuse(w, r, "could not start a session")
		return
	}

	// Kept for a sign-out, and only for that. Replaced rather than appended,
	// so signing in again on another machine does not leave a stale one.
	if who.Token != "" {
		h.keep(user.ID, who.Token)
	}

	// Cleared, because it has been spent. A state that stays valid is a
	// sign-in somebody can replay.
	h.forget(state.ID)
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.secure(r), SameSite: http.SameSiteLaxMode,
	})

	h.log.Info("signed in", "user", user.ID, "provider", h.flow.Name(),
		"project", user.Org+"/"+user.Project, "role", user.Role)
	audit(r.Context(), h.log, principal.Principal{
		Subject: user.ID, Email: user.Email, OrgID: user.Org, ProjectID: user.Project,
	}, ActionSignIn, user.Email, Allowed,
		map[string]any{"provider": h.flow.Name(), "role": user.Role})

	/*
	   Handed to the page in the URL fragment, which the browser does not send
	   to any server and which does not reach a proxy log or a Referer header.

	   A cookie would be tidier and is not available: the portal is a static
	   build that may be served from another origin entirely, and its API calls
	   carry a bearer token rather than credentials. The page takes the token
	   out of the fragment and replaces the history entry, so it survives one
	   render and no more.
	*/
	landing := safeReturn(who.Returning)
	http.Redirect(w, r, landing+"#token="+url.QueryEscape(issued), http.StatusFound)
}

// adopt places the person the provider vouched for.
//
// Where the provider said, or where this deployment puts everybody. A
// directory that could not name an organisation is not a reason to refuse
// somebody the identity provider just authenticated — it is a deployment that
// has one project, which is most of them.
func (h *SSO) adopt(ctx context.Context, who extension.Identity) (identity.User, error) {
	user := identity.User{
		// Namespaced by the provider, so a subject that collides with a local
		// account is not the same person by accident.
		ID:      "sso_" + h.flow.Name() + "_" + who.Subject,
		Email:   who.Email,
		Name:    who.Name,
		Org:     firstOf(who.Org, h.org),
		Project: firstOf(who.Project, h.project),
		Role:    firstOf(who.Role, h.role),
	}
	if h.users == nil {
		// No directory to record them in — a file-backed deployment. The
		// session still works; there is simply nothing to disable later.
		return user, nil
	}
	return h.users.Upsert(ctx, user)
}

// refuse sends the browser back to sign-in with something to read.
//
// A redirect rather than JSON: this route is reached by a person, not by a
// script, and a page of JSON is not an answer to somebody who clicked a button.
func (h *SSO) refuse(w http.ResponseWriter, r *http.Request, why string) {
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.secure(r), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/?sso_error="+url.QueryEscape(why), http.StatusFound)
}

func (h *SSO) remember(s extension.State) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Swept here rather than on a timer: this map is only touched on this
	// path, and an abandoned sign-in is a few hundred bytes until it expires.
	now := h.now()
	for id, held := range h.states {
		if now.After(held.Expires) {
			delete(h.states, id)
		}
	}
	h.states[s.ID] = s
}

func (h *SSO) recall(id string) (extension.State, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	s, ok := h.states[id]
	if !ok || h.now().After(s.Expires) {
		delete(h.states, id)
		return extension.State{}, false
	}
	return s, true
}

// hint is one session's identity token and how long it is worth keeping.
type hint struct {
	token   string
	expires time.Time
}

// keep holds a hint for as long as the session it was minted alongside.
func (h *SSO) keep(user, token string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Swept on the way in, like states. A person who signs in and closes the
	// tab never calls the sign-out route, so nothing else would ever remove
	// theirs, and a long-lived process would accumulate one provider
	// credential per person who has ever signed in.
	now := h.now()
	for id, held := range h.hints {
		if now.After(held.expires) {
			delete(h.hints, id)
		}
	}
	h.hints[user] = hint{token: token, expires: now.Add(SessionLifetime)}
}

// spend takes a hint and removes it: it is good for one sign-out.
func (h *SSO) spend(user string) string {
	h.mu.Lock()
	defer h.mu.Unlock()

	held, ok := h.hints[user]
	delete(h.hints, user)
	if !ok || h.now().After(held.expires) {
		return ""
	}
	return held.token
}

func (h *SSO) forget(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.states, id)
}

/*
safeReturn keeps a redirect inside this portal.

A path, and only a path. Anything with a scheme or a host — including the
protocol-relative `//evil.example`, which a naive check misses — is an open
redirect on the one route somebody arrives at expecting to have just proved who
they are, and the page they land on can ask for a password convincingly.
*/
func safeReturn(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	if strings.ContainsAny(raw, "\\\r\n") {
		return "/"
	}
	return raw
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

/*
logout ends the provider's session as well as this one.

The local session is a signed token with no server-side record, so ending it is
the browser's business and has already happened by the time this is called:
what this answers is where to go next, which is the provider's end-session
endpoint or nothing.

Answered as JSON rather than as a redirect, because the page is a script that
has just cleared its own storage and needs to decide whether to navigate. A
redirect here would be a redirect from a fetch, which goes nowhere.
*/
func (h *SSO) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, "Use POST.")
		return
	}

	out, ok := h.flow.(extension.SignOutFlow)
	if !ok {
		// This provider cannot end its own session. The local one is over
		// either way, and saying so is better than sending somebody to a URL
		// nobody published.
		send(w, http.StatusOK, map[string]string{})
		return
	}

	// The hint belongs to whoever is asking, which needs a session — so this
	// route is the one place a sign-out is authenticated. Without one there is
	// nothing to end and nothing to look up.
	var token string
	if pr, signedIn := h.auth.Principal(r); signedIn {
		token = h.spend(pr.Subject)

		h.log.Info("signed out", "user", pr.Subject, "provider", h.flow.Name())
		audit(r.Context(), h.log, pr, ActionSignOut, pr.Subject, Allowed,
			map[string]any{"provider": h.flow.Name()})
	}

	// Nothing from the request decides where this goes. The provider will only
	// accept a URL registered with it, so the landing is the deployment's
	// configuration — which is also what keeps an open redirect off the one
	// route somebody reaches expecting to have just left.
	send(w, http.StatusOK, map[string]string{"redirect": out.SignOut(token)})
}

// SSOClock moves this handler's clock, for tests that need a session to have
// expired without waiting eight hours for it.
func SSOClock(h *SSO, now func() time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.now = now
}
