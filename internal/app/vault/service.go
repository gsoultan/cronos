package vault

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/secret"
)

// Limits on what one project may store. Generous for anything a report needs —
// a service account's JSON is a few kilobytes — and small enough that the table
// cannot be used as somewhere to keep files.
const (
	MaxSecrets    = 256
	MaxValueBytes = 16 << 10
	maxNameBytes  = 128
)

/*
Service keeps one project's secrets and answers ${secret:…} for it.

It is the project's secret.Resolver: its own stored secrets first, then the
deployment's as far as the deployment shares them. Stored first, because a
project that set its own Mapbox token means that one rather than whatever the
operator put in the environment for everybody.
*/
type Service struct {
	store  Store
	sealer Sealer
	// where is the tenancy this project answers for, asked rather than
	// remembered: a first run can rename a single-project deployment while it
	// is running, and a resolver holding the old name would read nothing.
	where  func() (org, project string)
	shared secret.Resolver
	defs   Definitions
	cache  *Cache
	log    *slog.Logger
	now    func() time.Time

	// changed hears about every secret that changed, here or on another
	// replica, with the datasources that read it — so the connections built
	// from the old value can be rebuilt.
	changed func(name string, reads []definition.DataSource)

	mu sync.Mutex
	// seen is what Watch last read, by name. Written by Set and Delete too, so
	// this replica's own change is not heard about a second time.
	seen     map[string]time.Time
	watching bool
}

// New keeps secrets in store, sealed by sealer, for the project where names.
// A nil sealer is a deployment with no secrets key: nothing is stored or read
// from the store, and the deployment's own secrets still resolve.
func New(store Store, sealer Sealer, where func() (string, string), log *slog.Logger) *Service {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{
		store: store, sealer: sealer, where: where, log: log,
		now:  time.Now,
		seen: map[string]time.Time{},
	}
}

// WithShared falls back to the deployment's own secrets, as this project may
// see them.
func (s *Service) WithShared(r secret.Resolver) *Service {
	s.shared = r
	return s
}

// WithDefinitions says which secrets the project uses, and which of its
// datasources a changed secret belongs to.
func (s *Service) WithDefinitions(d Definitions) *Service {
	s.defs = d
	return s
}

// WithCache shares a cache between projects. Keyed by tenant, so sharing it is
// safe; see Cache.
func (s *Service) WithCache(c *Cache) *Service {
	s.cache = c
	return s
}

// OnChange is told the name of every secret that changes, and which of the
// project's datasources read it.
func (s *Service) OnChange(fn func(name string, reads []definition.DataSource)) *Service {
	s.changed = fn
	return s
}

// Available reports whether this deployment can store a secret at all.
func (s *Service) Available() bool { return s.store != nil && s.sealer != nil }

// may checks the caller is an editor of the project this Service keeps.
//
// The tenancy too, although whoever resolved this Service from the caller
// already checked it: a Service handed the wrong principal must refuse rather
// than write into the project it happens to hold.
func (s *Service) may(pr principal.Principal) error {
	org, project := s.where()
	if pr.OrgID != org || pr.ProjectID != project || !pr.CanEdit() {
		return ErrForbidden
	}
	return nil
}

func (s *Service) notify(name string) {
	if s.changed != nil {
		s.changed(name, s.usesOf(name))
	}
}
