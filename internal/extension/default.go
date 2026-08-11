package extension

import (
	"context"
	"net/http"
)

// defaultAuth fails closed. The core registers a local username/password
// provider at startup once internal/auth/local exists; until then an
// unconfigured deployment refuses requests rather than serving them anonymously.
type defaultAuth struct{}

func (defaultAuth) Name() string { return "none" }

func (defaultAuth) Authenticate(context.Context, *http.Request) (*Principal, error) {
	return nil, ErrNotConfigured
}

// discardAudit is the default sink. Dropping audit events is a legitimate
// choice for a single-tenant internal deployment; ee/audit persists them.
type discardAudit struct{}

func (discardAudit) Name() string { return "discard" }

func (discardAudit) Record(context.Context, Event) error { return nil }
