package boot

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/api"
	"github.com/gsoultan/cronos/internal/adapter/driver/registry"
	sqlstore "github.com/gsoultan/cronos/internal/adapter/store/sql"
	"github.com/gsoultan/cronos/internal/app/vault"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/platform/config"
	"github.com/gsoultan/cronos/internal/platform/secret"
)

/*
Each project's secrets: what it stored through the portal, and the deployment's
own behind them.

One vault per runtime, and it is that runtime's secret.Resolver for everything
— the datasources it opens and the tile keys its maps send — so a password typed
into the datasource wizard is what the connection opens with, without anybody
touching the environment or restarting anything.
*/

const (
	// secretsCached is how long an answer is remembered before the store is
	// asked again. Watch catches another replica's change sooner; this bounds
	// how stale an answer can be if Watch cannot reach the store.
	secretsCached = time.Minute
	// secretsHeld bounds the answers remembered across every project.
	secretsHeld = 4096
	// secretsWatched is how often each replica looks for a change another one
	// made. Rotating a warehouse password reaches every replica inside this.
	secretsWatched = 30 * time.Second
)

// sealing builds what seals stored secrets, or nothing where no key is set. A
// key that is set and too weak is an error at startup, like the signing key.
//
// An interface holding nil rather than a nil *secret.Sealer: vault asks
// whether it was given one, and a typed nil answers yes.
func sealing(cfg config.Server) (vault.Sealer, error) {
	if len(cfg.SecretsKey) == 0 {
		return nil, nil
	}
	s, err := secret.NewSealer(cfg.SecretsKey, cfg.PreviousSecretsKeys...)
	if err != nil {
		return nil, fmt.Errorf("CRONOS_SECRETS_KEY: %w", err)
	}
	return s, nil
}

/*
deploymentSecrets is the deployment's own secrets as one project may read them.

All of them where there is one project, which is every deployment this changes
nothing for. Where there are several, each reads its own directory under the
secrets directory, and only the names CRONOS_SHARED_SECRETS lists from the rest
— see secret.Only for what happened before.
*/
func deploymentSecrets(cfg config.Server, t tenant, several bool) secret.Resolver {
	if !several {
		return secrets(cfg)
	}
	shared := map[string]bool{}
	for _, name := range cfg.SharedSecrets {
		shared[name] = true
	}
	var own secret.Resolver
	if cfg.SecretsDir != "" {
		own = secret.Files{Dir: filepath.Join(cfg.SecretsDir, t.org, t.project)}
	}
	return secret.Chain{own, secret.Only{Names: shared, From: secrets(cfg)}}
}

// keeping builds a runtime's vault. Without a records store it stores nothing
// and resolves the deployment's secrets exactly as before it existed.
func keeping(cfg config.Server, rt *runtime, records *sqlstore.Store, sealer vault.Sealer,
	cache *vault.Cache, several bool, log *slog.Logger) *vault.Service {

	var store vault.Store
	if records != nil {
		store = records
	}
	return vault.New(store, sealer, rt.serving, log).
		WithShared(deploymentSecrets(cfg, rt.tenant, several)).
		WithDefinitions(rt.repo).
		WithCache(cache)
}

/*
reopen rebuilds the connections a changed secret was used to open.

A pool holds the DSN it was opened with, so a rotated password is not used until
the source is opened again — and a source that would not open because its
password was never set stays shut until something tries. Adopt is exactly the
work publishing an edited datasource does.
*/
func reopen(rt *runtime, log *slog.Logger) func(string, []definition.DataSource) {
	return func(name string, reads []definition.DataSource) {
		reg, ok := rt.project.Probes.(*registry.Registry)
		if !ok || reg == nil {
			return
		}
		for _, ds := range reads {
			if err := reg.Adopt(ds); err != nil {
				log.Error("datasource not reopened after its secret changed — reports that read it will fail",
					"project", rt.tenant.String(), "source", ds.Name, "secret", name, "err", err)
				unopenable.Add(1)
				continue
			}
			log.Info("datasource reopened", "project", rt.tenant.String(),
				"source", ds.Name, "secret", name)
		}
	}
}

/*
reseal moves every stored secret onto the current key, at startup.

Only where there is a store and a key, and loud about what no configured key can
open: that is a key replaced without keeping the old one, and otherwise it is
found one broken map at a time.
*/
func reseal(ctx context.Context, records *sqlstore.Store, sealer vault.Sealer, log *slog.Logger) error {
	if records == nil {
		return nil
	}
	if sealer == nil {
		log.Info("secrets", "store", false,
			"why", "CRONOS_SECRETS_KEY is not set, so the portal cannot store a secret")
		return nil
	}
	resealed, lost, err := vault.Reseal(ctx, records, sealer)
	if err != nil {
		return fmt.Errorf("resealing stored secrets: %w", err)
	}
	if len(lost) > 0 {
		log.Error("stored secrets no configured key can open — was CRONOS_SECRETS_KEY replaced "+
			"without keeping the old one in CRONOS_SECRETS_KEY_PREVIOUS?", "secrets", lost)
	}
	log.Info("secrets", "store", true, "resealed", resealed)
	return nil
}

// keepSecrets is what every project's vault shares: the sealer, with anything
// a retired key sealed moved onto the current one, and one cache keyed by
// project — see vault.Cache.
func keepSecrets(ctx context.Context, cfg config.Server, records *sqlstore.Store,
	log *slog.Logger) (vault.Sealer, *vault.Cache, error) {

	sealer, err := sealing(cfg)
	if err != nil {
		return nil, nil, err
	}
	if err := reseal(ctx, records, sealer, log); err != nil {
		return nil, nil, err
	}
	return sealer, vault.NewCache(secretsCached, secretsHeld), nil
}

/*
follow points each runtime's secrets at the tenancy the API answers with, and
starts watching for changes another replica makes. It returns what stops them.

A first run can rename a single-project deployment while it runs; a resolver
still reading the old name would find none of the secrets set under the new one.
*/
func follow(projects api.Projects, runtimes map[tenant]*runtime) func() {
	if one, ok := projects.(*api.One); ok {
		for _, rt := range runtimes {
			rt.serve = one.Serving
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	for _, rt := range runtimes {
		go rt.vault.Watch(ctx, secretsWatched)
	}
	return cancel
}

// secretsFor serves the API from the caller's own project's vault, resolved the
// way every other per-project service is — see publishingFor.
func secretsFor(records *sqlstore.Store, sealer vault.Sealer, projects api.Projects,
	runtimes map[tenant]*runtime) api.Secrets {

	if records == nil {
		return nil
	}
	byProject := map[*api.Project]*vault.Service{}
	for _, rt := range runtimes {
		byProject[rt.project] = rt.vault
	}
	return secretsPerProject{projects: projects, byProject: byProject, storing: sealer != nil}
}

type secretsPerProject struct {
	projects  api.Projects
	byProject map[*api.Project]*vault.Service
	storing   bool
}

func (s secretsPerProject) of(ctx context.Context, pr principal.Principal) (*vault.Service, error) {
	project, err := s.projects.Project(ctx, pr)
	if err != nil {
		return nil, fmt.Errorf("%w: no such project here", vault.ErrForbidden)
	}
	v, ok := s.byProject[project]
	if !ok {
		return nil, fmt.Errorf("%w: no secrets here", vault.ErrForbidden)
	}
	return v, nil
}

func (s secretsPerProject) Storing() bool { return s.storing }

func (s secretsPerProject) List(ctx context.Context, pr principal.Principal) ([]vault.Entry, error) {
	v, err := s.of(ctx, pr)
	if err != nil {
		return nil, err
	}
	return v.List(ctx, pr)
}

func (s secretsPerProject) Set(ctx context.Context, pr principal.Principal, name, value string) error {
	v, err := s.of(ctx, pr)
	if err != nil {
		return err
	}
	return v.Set(ctx, pr, name, value)
}

func (s secretsPerProject) Delete(ctx context.Context, pr principal.Principal, name string) error {
	v, err := s.of(ctx, pr)
	if err != nil {
		return err
	}
	return v.Delete(ctx, pr, name)
}
