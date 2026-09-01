// Package extension defines the seams where commercially-licensed features plug
// into the core.
//
// Every seam has a working default, so a build without ee/ is a complete
// product rather than a crippled one. Embedding and multi-tenancy are
// deliberately not seams: the license restricts those, not the code.
//
// Implementations register themselves from init(), so the import graph is the
// license boundary. cmd/cronosd must never reach ee/; scripts/check-license-boundary.sh
// enforces that in CI.
package extension

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/core/principal"
)

// ErrNotConfigured is returned by a default implementation that cannot serve a
// request without being configured first. Seams fail closed.
var ErrNotConfigured = errors.New("extension: not configured")

/*
Why a sign-in did not finish, in the only terms the person reading it can act on.

A sign-in through a directory fails in half a dozen ways and until this they all
reached the browser as "the identity provider refused this sign-in". Only one of
them is that. The rest are cronos refusing a token, or a deployment configured
against a different client, or two machines disagreeing about the time — and
sending an operator to the provider's admin console for any of those costs them
the afternoon. It cost one here: a host clock that jumped hours put a valid
token outside its window, the log said the provider refused, and the provider
was the first thing restarted.

Wrapped by whichever implementation is registered, read by the callback handler
in internal/adapter/api. The detail stays in the log; these decide which system
the sentence names.
*/
var (
	// ErrProviderRefused is the provider saying no — a consent screen somebody
	// declined, or an account it will not sign in. The only one where the
	// provider is the right place to look.
	ErrProviderRefused = errors.New("extension: the provider refused the sign-in")

	// ErrClockSkew is a token outside its validity window. Almost always the
	// two machines rather than the token: a directory and an application that
	// disagree about the time by more than the minute of leeway allowed.
	ErrClockSkew = errors.New("extension: the token is outside its validity window")

	// ErrNotAcceptable is a sign-in that worked and a person this deployment
	// will not have — an address outside the permitted domains. Nothing is
	// broken and nothing will fix itself; somebody has to be added.
	ErrNotAcceptable = errors.New("extension: this deployment does not accept that account")
)

// Principal is the identity and active scope a request runs as. See
// internal/core/principal and docs/tenancy.md.
type Principal = principal.Principal

/*
AuthProvider establishes the Principal for an inbound request, including the
organization and project it is acting in.

Per request, which is the shape a reverse proxy or a host application wants: it
has already authenticated somebody against an identity provider and puts the
result on every call. Nothing here manages a session, because in that model
there is not one to manage.

A browser signing in is a different act and needs SignInFlow.
*/
type AuthProvider interface {
	Name() string
	Authenticate(ctx context.Context, r *http.Request) (*Principal, error)
}

/*
SignInFlow redirects a browser to an identity provider and receives it back.

The seam AuthProvider could not express. That one answers "who is this
request", which is a question with an answer only once somebody has already
signed in somewhere; a person opening the portal for the first time has no
token to present, and what they need is to be sent to Okta and returned with
one.

Two calls because the browser leaves in between. Start hands back somewhere to
go and the opaque state that must come back with them; Complete reads the
answer and says who they are. Everything in between is the provider's own
business — this interface deliberately mentions no protocol, so SAML could
implement it without OIDC's vocabulary leaking into the core.
*/
type SignInFlow interface {
	Name() string
	// Start returns where to send the browser, and the state to hold until it
	// comes back. `returning` is where the portal wants the person to land.
	Start(ctx context.Context, returning string) (redirect string, state State, err error)
	// Complete reads the identity provider's answer.
	Complete(ctx context.Context, r *http.Request, state State) (Identity, error)
}

/*
SignOutFlow ends the session at the identity provider as well as here.

Optional, and asked for by type rather than required, because not every
provider supports it and a flow that cannot do it should not have to pretend.

Without it, signing out ends the cronos session and nothing else: the person is
still signed in where they thought they had left, and the next sign-in is
silent — which reads as "the log-out button does not work", and on a shared
machine is somebody else's session.
*/
type SignOutFlow interface {
	/*
	   SignOut returns where to send the browser to end the provider's session.

	   `hint` is the identity token this session was minted from, when the core
	   still has it. Some providers require it and refuse without one; others
	   accept the client id alone. Empty means it is gone — a restart drops
	   them — and the provider decides what to do about that.

	   An empty redirect means this provider has no such endpoint, and the
	   caller ends the local session and says nothing more.

	   Where the browser lands afterwards is deliberately not a parameter. It
	   must be a URL registered with the provider, an unregistered one is
	   refused outright, and the caller's idea of "where I was" is a path inside
	   the portal — so it is the provider's own configuration, and nothing a
	   request can influence.
	*/
	SignOut(hint string) string
}

// State is what a sign-in leaves behind while the browser is away.
//
// Opaque to the core, which stores it against a cookie and hands it back. A
// provider puts whatever the round trip needs in it — a nonce, a PKCE
// verifier, where to return to — and the core never reads any of it.
type State struct {
	// ID is what the core keys it by, and what the browser carries.
	ID string
	// Data is the provider's own, serialised however it likes.
	Data map[string]string
	// Expires bounds the round trip. A sign-in somebody abandoned is not a
	// sign-in that should still complete tomorrow.
	Expires time.Time
}

// Identity is who the provider says this is.
//
// Deliberately not a Principal: a provider knows what its directory says, and
// how that becomes an organisation, a project and a role is the deployment's
// mapping rather than the provider's opinion.
type Identity struct {
	// Subject is stable and opaque — the provider's own id for the person, not
	// their email, which they can change.
	Subject string
	Email   string
	Name    string
	// Groups are whatever the provider asserts. A deployment maps them to a
	// role; cronos does not guess.
	Groups []string
	// Org, Project and Role are set when the provider resolved them from its
	// own configuration. Empty means the deployment's defaults apply.
	Org     string
	Project string
	Role    string
	// Returning is where the browser asked to land, carried through.
	Returning string
	/*
	   Token is what the provider signed, kept only so a later sign-out can
	   present it back as an id_token_hint.

	   It is not stored anywhere durable and never reaches the browser: a
	   provider's identity token is a credential at that provider, and putting
	   one in a database or a page would hand somebody a way in that has
	   nothing to do with cronos.
	*/
	Token string
}

// Event is one auditable action. Report runs, definition changes, and delivery
// attempts all emit one.
//
// Org and project are recorded separately rather than folded into one scope
// string: "who was billed for this" and "which resources it touched" are
// different questions, and an audit that cannot separate them cannot answer
// either.
type Event struct {
	At        time.Time
	Actor     string
	OrgID     string
	ProjectID string
	Action    string
	Target    string
	Result    string
	Detail    map[string]any
}

// AuditSink durably records Events.
type AuditSink interface {
	Name() string
	Record(ctx context.Context, e Event) error
}

var (
	mu    sync.RWMutex
	auth  AuthProvider = defaultAuth{}
	audit AuditSink    = discardAudit{}
)

// RegisterAuthProvider installs p as the process-wide AuthProvider. It panics
// if called twice: two providers means the wiring is wrong, and silently
// picking one would decide an authentication question by import order.
func RegisterAuthProvider(p AuthProvider) {
	mu.Lock()
	defer mu.Unlock()
	if _, isDefault := auth.(defaultAuth); !isDefault {
		panic("extension: AuthProvider already registered by " + auth.Name())
	}
	auth = p
}

// SignIn returns the registered SignInFlow, or nil when there is none.
//
// Nil rather than a no-op default: a route that exists and always refuses is
// one somebody spends an afternoon probing, so the endpoints are not mounted
// at all where no provider registered.
func SignIn() SignInFlow {
	mu.RLock()
	defer mu.RUnlock()
	return flow
}

// RegisterSignInFlow installs f as the process-wide SignInFlow.
func RegisterSignInFlow(f SignInFlow) {
	mu.Lock()
	defer mu.Unlock()
	if flow != nil {
		panic("extension: SignInFlow already registered by " + flow.Name())
	}
	flow = f
}

var flow SignInFlow

// RegisterAuditSink installs s as the process-wide AuditSink.
func RegisterAuditSink(s AuditSink) {
	mu.Lock()
	defer mu.Unlock()
	if _, isDiscard := audit.(discardAudit); !isDiscard {
		panic("extension: AuditSink already registered by " + audit.Name())
	}
	audit = s
}

// Auth returns the active AuthProvider.
func Auth() AuthProvider {
	mu.RLock()
	defer mu.RUnlock()
	return auth
}

// Audit returns the active AuditSink.
func Audit() AuditSink {
	mu.RLock()
	defer mu.RUnlock()
	return audit
}
