package boot

import (
	"context"

	"github.com/gsoultan/cronos/internal/adapter/driver/registry"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
)

/*
One project's datasources, and which of them answers a dataset.

A thin thing on purpose: the registry does the work, and this exists only to
hold the one rule the registry has no opinion about — that a deployment with no
sources at all reads the configured database, and a deployment with any source
reads the registry, so naming a source that is not there is an error rather
than a silent read of the development database.

That rule used to be expressed by building one or the other at startup, which
made it permanent: a deployment that booted without sources answered every
dataset from CRONOS_DSN for the life of the process, however many warehouses
were connected afterwards.
*/
type sources struct {
	registry *registry.Registry
	// configured is CRONOS_DSN, and nil unless the deployment started with no
	// sources defined. Nil is not "no fallback available" by accident: a
	// deployment that defined sources at startup never opened this pool, and
	// falling back to one it did not open is a connection nobody configured.
	configured run.Engines
}

/*
Engine resolves the engine for a dataset.

The registry, unless nothing is registered at all. Empty is asked per call and
not remembered, because the whole point is that the answer changes the moment
somebody connects their first source.
*/
func (s *sources) Engine(ctx context.Context, ds definition.Dataset) (run.Engine, error) {
	if s.configured != nil && s.registry.Empty() {
		return s.configured.Engine(ctx, ds)
	}
	return s.registry.Engine(ctx, ds)
}

/*
probes is what answers "is this source there", for the test endpoint, the
readiness checks and the pool metrics.

The registry itself rather than this wrapper, so all three read the live map:
a source connected this afternoon is testable, probed and counted without a
restart, which is the half of adopting one that is easy to leave out.
*/
func (s *sources) probes() *registry.Registry { return s.registry }
