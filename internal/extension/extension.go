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
