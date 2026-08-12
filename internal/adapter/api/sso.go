package api

import (
	"context"
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
	log    *slog.Logger

	// Where the deployment's people belong when the provider does not say.
	org, project, role string

	mu     sync.Mutex
	states map[string]extension.State
	now    func() time.Time
}

// Directory is how a person the provider vouched for becomes somebody here.
type Directory interface {
	Upsert(ctx context.Context, u identity.User) (identity.User, error)
}

// NewSSO wires the handler.
func NewSSO(f extension.SignInFlow, s *token.Signer, d Directory, log *slog.Logger) *SSO {
	return &SSO{
		flow: f, signer: s, users: d, log: log,
		states: map[string]extension.State{},
		now:    time.Now,
	}
}

// In names where people land when the provider does not say.
func (h *SSO) In(org, project, role string) *SSO {
	h.org, h.project, h.role = org, project, role
	return h
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
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode,
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
		h.log.Info("sign-in refused by the provider", "provider", h.flow.Name(), "err", err)
		audit(r.Context(), h.log, principal.Principal{Subject: "sso"},
			ActionSignIn, "sso", Refused, map[string]any{"provider": h.flow.Name()})
		h.refuse(w, r, "the identity provider refused this sign-in")
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
	}, SessionLifetime)
	if err != nil {
		h.log.Error("could not mint a session", "err", err)
		h.refuse(w, r, "could not start a session")
		return
	}

	// Cleared, because it has been spent. A state that stays valid is a
	// sign-in somebody can replay.
	h.forget(state.ID)
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode,
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
		Name: stateCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
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
