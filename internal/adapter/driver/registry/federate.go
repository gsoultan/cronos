package registry

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gsoultan/cronos/internal/adapter/driver/duckdb"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
)

/*
federation is one DuckDB connection with a dataset's sources mounted.

Kept rather than built per query. Mounting attaches somebody else's warehouse,
and a burst that does it once per report is a burst their DBA sees — which is
the reason duckdb.Open mounts everything at open and nothing per query, and the
reason that would be worth nothing if this opened one each time.
*/
type federation struct {
	// once so that a burst arriving at a cold dataset mounts once and waits,
	// rather than opening one connection per concurrent report.
	once   sync.Once
	fed    *duckdb.Federation
	limits definition.Limits
	err    error
}

/*
federate resolves the engine for a dataset that needs more than one source, or
an object store — the two cases a single database connection cannot answer.

The registry lock is not held across the open. Mounting reaches other people's
databases and can take as long as they take, and holding a registry-wide lock
through it would queue every unrelated dataset behind the slowest warehouse.
*/
func (r *Registry) federate(ctx context.Context, ds definition.Dataset) (run.Engine, error) {
	mounts, limits, err := r.mounts(ds)
	if err != nil {
		return run.Engine{}, err
	}
	key := fingerprint(mounts)

	r.fedMu.Lock()
	if r.closed {
		r.fedMu.Unlock()
		return run.Engine{}, fmt.Errorf("%w: dataset %q", ErrClosed, ds.Name)
	}
	f, ok := r.federations[key]
	if !ok {
		f = &federation{limits: limits}
		r.federations[key] = f
	}
	r.fedMu.Unlock()

	f.once.Do(func() { f.fed, f.err = duckdb.Open(ctx, mounts) })

	if f.err != nil {
		/*
		   A failed open is not remembered.

		   once would otherwise make the first failure permanent: a warehouse
		   that was restarting, or a caller that gave up mid-mount, would take
		   the dataset out until somebody restarted cronos. Dropping the entry
		   means the next report mounts again — and the identity check is so
		   that a goroutine still holding the failed one cannot delete the
		   replacement somebody else already made.
		*/
		r.fedMu.Lock()
		if r.federations[key] == f {
			delete(r.federations, key)
		}
		r.fedMu.Unlock()

		if errors.Is(f.err, duckdb.ErrNotBuilt) {
			return run.Engine{}, fmt.Errorf("%w: dataset %q reads %s — %s",
				ErrNoFederation, ds.Name, describe(mounts), f.err)
		}
		return run.Engine{}, fmt.Errorf("federating dataset %q: %w", ds.Name, f.err)
	}

	return run.Engine{
		Executor: f.fed.Executor().WithLimits(f.limits),
		// DuckDB takes Postgres placeholders, which is what dialectFor already
		// says for the duckdb driver.
		Builder: query.NewBuilder(query.Postgres{}),
	}, nil
}

/*
mounts is what each source is called in the dataset's query, and the bound the
whole thing runs under.

Resolved before anything is opened, so a dataset naming a source that is not
registered fails with that sentence rather than with whatever DuckDB says about
a table it cannot find.
*/
func (r *Registry) mounts(ds definition.Dataset) (
	map[string]definition.DataSource, definition.Limits, error) {

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]definition.DataSource, len(ds.Sources))
	defs := make([]definition.DataSource, 0, len(ds.Sources))
	for _, ref := range ds.Sources {
		s, ok := r.sources[ref.Ref]
		if !ok {
			return nil, definition.Limits{}, fmt.Errorf("%w: dataset %q reads %q",
				ErrUnknownSource, ds.Name, ref.Ref)
		}
		name := ref.Name()
		if !duckdb.MountName(name) {
			// A datasource name is a slug and may hold dashes. A mount name is
			// an identifier in a statement and may not, and `as:` is how a
			// dataset spells the difference.
			return nil, definition.Limits{}, fmt.Errorf(
				"%w: dataset %q calls a source %q, which a query cannot name — add `as:` to that source",
				ErrBadMountName, ds.Name, name)
		}
		if _, dup := out[name]; dup {
			// Two sources under one name is a query whose joins mean something
			// its author did not write, and a map that silently keeps one.
			return nil, definition.Limits{}, fmt.Errorf(
				"%w: dataset %q calls two sources %q", ErrBadMountName, ds.Name, name)
		}
		out[name] = s.def
		defs = append(defs, s.def)
	}
	return out, tightest(defs), nil
}

/*
tightest is the most restrictive bound among the sources being read.

Each of those limits is somebody's answer about their own database. A
federation reads several at once, and taking the loosest would let a query
permitted five minutes against a warehouse hold open a lake whose operator
allowed thirty seconds.
*/
func tightest(defs []definition.DataSource) definition.Limits {
	var timeout time.Duration
	var rows int
	// Resolved values, not the raw fields: a source that set neither means the
	// default, and comparing the zero it stores would make it the tightest.
	for i, d := range defs {
		if i == 0 || d.Limits.Timeout() < timeout {
			timeout = d.Limits.Timeout()
		}
		if i == 0 || d.Limits.Rows() < rows {
			rows = d.Limits.Rows()
		}
	}
	return definition.Limits{
		StatementTimeout: definition.Duration(timeout),
		MaxRows:          rows,
	}
}

// fingerprint identifies a set of mounts, so two datasets reading the same
// sources under the same names share one connection rather than attaching
// those databases twice.
func fingerprint(mounts map[string]definition.DataSource) string {
	parts := make([]string, 0, len(mounts))
	for name, def := range mounts {
		parts = append(parts, name+"="+def.Name)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// describe names the sources in a message, in a stable order.
func describe(mounts map[string]definition.DataSource) string {
	parts := make([]string, 0, len(mounts))
	for _, def := range mounts {
		parts = append(parts, named(def))
	}
	sort.Strings(parts)
	return strings.Join(parts, " and ")
}

func named(d definition.DataSource) string {
	if d.Federated() {
		return fmt.Sprintf("%q (an object store)", d.Name)
	}
	return fmt.Sprintf("%q", d.Name)
}

// closeFederations releases every mounted connection.
func (r *Registry) closeFederations() error {
	r.fedMu.Lock()
	defer r.fedMu.Unlock()

	r.closed = true
	var first error
	for _, f := range r.federations {
		// once, so a federation still being opened by another goroutine is
		// waited for rather than left behind holding an attachment.
		f.once.Do(func() {})
		if f.fed == nil {
			continue
		}
		if err := f.fed.Close(); err != nil && first == nil {
			first = err
		}
	}
	r.federations = nil
	return first
}

/*
Mounted is how many federations are currently held open.

Operational rather than diagnostic. Each one is a set of attachments to
databases somebody else runs, so it is a number their DBA is effectively also
watching — and the thing to alert on is growth, which would mean datasets are
mounting per query rather than sharing.
*/
func (r *Registry) Mounted() int {
	r.fedMu.Lock()
	defer r.fedMu.Unlock()
	return len(r.federations)
}
