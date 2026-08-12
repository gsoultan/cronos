package registry

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	sqldriver "github.com/gsoultan/cronos/internal/adapter/driver/sql"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/query"
	"github.com/gsoultan/cronos/internal/platform/secret"
)

// source is one opened datasource.
type source struct {
	def     definition.DataSource
	db      *sql.DB
	dialect query.Dialect
}

// Registry holds a connection per datasource.
type Registry struct {
	mu      sync.RWMutex
	sources map[string]*source
	log     *slog.Logger
}

// New opens every datasource.
//
// At startup, not per query. A connection to somebody else's warehouse is a
// thing they see in their monitoring, and opening one per report is how a
// reporting tool becomes the reason their connection count alarms.
//
// A source that will not open is fatal rather than skipped: a server that
// starts with three of its four warehouses unreachable serves three-quarters
// of its reports and fails the rest at six in the morning.
func New(defs []definition.DataSource, secrets secret.Resolver, log *slog.Logger) (*Registry, error) {
	r := &Registry{sources: map[string]*source{}, log: log}

	for _, def := range defs {
		if def.Federated() {
			// An object store is not connected to; it is read through an
			// engine that can address files. It is registered so a dataset can
			// name it, and resolving one is what needs federation.
			//
			// Its URI is resolved all the same: a bucket URL can carry a
			// reference, and one left as literal ${secret:…} text becomes a
			// path the reader looks for and does not find.
			uri, err := secret.Resolve(def.URI, secrets)
			if err != nil {
				r.Close()
				return nil, fmt.Errorf("datasource %q: %w", def.Name, err)
			}
			def.URI = uri
			r.sources[def.Name] = &source{def: def}
			continue
		}
		s, err := open(def, secrets)
		if err != nil {
			r.Close()
			return nil, err
		}
		r.sources[def.Name] = s
		log.Info("datasource", "name", def.Name, "driver", def.Driver,
			"timeout", def.Limits.Timeout(), "maxRows", def.Limits.Rows())
	}
	return r, nil
}

func open(def definition.DataSource, secrets secret.Resolver) (*source, error) {
	dialect, err := dialectFor(def.Driver)
	if err != nil {
		return nil, fmt.Errorf("datasource %q: %w", def.Name, err)
	}

	// The password arrives here and goes no further. It is resolved from the
	// reference the definition carries, handed to the driver, and never
	// stored, returned or logged — the definition on disk still says
	// ${secret:name}, which is the only version anything else ever sees.
	dsn, err := secret.Resolve(def.DSN, secrets)
	if err != nil {
		return nil, fmt.Errorf("datasource %q: %w", def.Name, err)
	}
	db, err := sql.Open(sqlDriver(def.Driver), dsn)
	if err != nil {
		// The definition's own text, not the resolved one: a driver error
		// quotes the string it was given, and by then it has a password in it.
		return nil, fmt.Errorf("datasource %q (%s): %w", def.Name, def.DSN, err)
	}

	// The pool is bounded because somebody else operates this database. A
	// reporting tool that opens two hundred connections during a burst is a
	// reporting tool their DBA turns off.
	if def.Pool.MaxOpen > 0 {
		db.SetMaxOpenConns(def.Pool.MaxOpen)
	}
	if def.Pool.MaxIdleTime > 0 {
		db.SetConnMaxIdleTime(time.Duration(def.Pool.MaxIdleTime))
	}
	return &source{def: def, db: db, dialect: dialect}, nil
}

// Engine resolves how to compile and run queries for a dataset.
func (r *Registry) Engine(_ context.Context, ds definition.Dataset) (run.Engine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	switch len(ds.Sources) {
	case 0:
		return run.Engine{}, fmt.Errorf("%w: %q", ErrNoSources, ds.Name)
	case 1:
		return r.single(ds, ds.Sources[0])
	}
	// More than one source in one query is a join across databases, and that
	// needs an engine that can hold both. Named rather than attempted, so the
	// message says what to build with instead of what could not be found.
	return run.Engine{}, fmt.Errorf("%w: dataset %q reads %d sources — rebuild with -tags duckdb",
		ErrNoFederation, ds.Name, len(ds.Sources))
}

func (r *Registry) single(ds definition.Dataset, ref definition.SourceRef) (run.Engine, error) {
	s, ok := r.sources[ref.Ref]
	if !ok {
		return run.Engine{}, fmt.Errorf("%w: dataset %q reads %q", ErrUnknownSource, ds.Name, ref.Ref)
	}
	if s.db == nil {
		// An object store on its own still needs an engine to read files.
		return run.Engine{}, fmt.Errorf("%w: %q is an object store — rebuild with -tags duckdb",
			ErrNoFederation, ref.Ref)
	}
	return run.Engine{
		Executor: sqldriver.NewExecutor(s.db).WithLimits(s.def.Limits),
		Builder:  query.NewBuilder(s.dialect),
	}, nil
}

// DB returns a source's connection.
//
// Exposed for the development seed and for nothing else. A deployment does not
// own the databases it reads — running DDL against somebody's warehouse
// because a flag was set is the last thing a reporting tool should be capable
// of, which is why the caller has to name the source explicitly.
func (r *Registry) DB(name string) (*sql.DB, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.sources[name]
	if !ok || s.db == nil {
		return nil, false
	}
	return s.db, true
}

// Only returns the name of the single connectable source, when there is one.
//
// So a development seed does not have to be told which of one databases to
// apply itself to, while more than one is ambiguous and says so rather than
// picking.
func (r *Registry) Only() (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	found := ""
	for name, s := range r.sources {
		if s.db == nil {
			continue
		}
		if found != "" {
			return "", false
		}
		found = name
	}
	return found, found != ""
}

// Names lists what is registered, for an error that can say what was available.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.sources))
	for name := range r.sources {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Close releases every connection.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var first error
	for _, s := range r.sources {
		if s.db == nil {
			continue
		}
		if err := s.db.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Probe opens a connection to the named source and asks it a question.
//
// Ping and a statement, not ping alone. Ping borrows a connection from the
// pool and may find one already open, which proves the pool remembers a
// database that has since gone away; a trivial select is a round trip the
// database has to be alive to complete.
//
// The duration is part of the answer. A source that responds in four seconds
// is one whose reports will time out under load, and "connected" alone would
// present that as healthy.
func (r *Registry) Probe(ctx context.Context, name string) (time.Duration, error) {
	db, ok := r.DB(name)
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownSource, name)
	}

	// Bounded here rather than left to the caller's context. A database that
	// accepts a connection and never answers would otherwise hold the request
	// open for as long as the browser waited.
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	started := time.Now()
	if err := db.PingContext(ctx); err != nil {
		return time.Since(started), err
	}
	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return time.Since(started), err
	}
	return time.Since(started), nil
}

// probeTimeout is how long a source has to prove it is there. Five seconds is
// long enough for a cold connection across a region and short enough that
// somebody waiting on the answer does not conclude the page is broken.
const probeTimeout = 5 * time.Second
