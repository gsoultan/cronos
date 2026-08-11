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

// AuthProvider establishes the Principal for an inbound request, including the
// organization and project it is acting in.
type AuthProvider interface {
	Name() string
	Authenticate(ctx context.Context, r *http.Request) (*Principal, error)
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
