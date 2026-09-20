package registry

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
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
	// secrets is kept because a source can arrive after New. A ${secret:…}
	// reference is resolved when the connection is opened, and one adopted at
	// four in the afternoon has to be resolved against the same backend the
	// ones opened at boot were.
	secrets secret.Resolver
	/*
	   unopened is why each defined source has no connection, by name.

	   Kept rather than logged, because whether that is fatal is the caller's
	   decision and a library that exits is a library nobody can embed. Keyed
	   by name rather than a list, and guarded, because a source can now fail
	   to open long after startup — an edited definition with a driver this
	   build has no import for replaces nothing and lands here — and because
	   the two questions asked of it are per source: is this one open, and
	   which ones are not.
	*/
	unopened map[string]error

	// federations are the mounted connections, keyed by the set of sources
	// they hold. Their own lock, because opening one reaches other people's
	// databases and must not queue every unrelated dataset behind it.
	fedMu       sync.Mutex
	federations map[string]*federation
	closed      bool
}

/*
Unavailable is every defined source this build could not open, and why.

Empty on a healthy deployment. Each one is a report that will fail with a
message naming it, and a number worth alerting on — see
cronos_datasources_unavailable.

Asked rather than remembered from startup. A source that would not open used
to be a snapshot taken during New, which was true while sources only ever
arrived from a directory; one published in the afternoon would not have been
in it, so nothing after boot could tell a deployment that a source it holds
has no connection behind it.
*/
func (r *Registry) Unavailable() []error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.unopened))
	for name := range r.unopened {
		names = append(names, name)
	}
	// Sorted, so a readiness answer and a startup log do not reorder
	// themselves between two reads of the same state.
	sort.Strings(names)

	out := make([]error, 0, len(names))
	for _, name := range names {
		out = append(out, r.unopened[name])
	}
	return out
}

// New opens every datasource.
//
// At startup, not per query. A connection to somebody else's warehouse is a
// thing they see in their monitoring, and opening one per report is how a
// reporting tool becomes the reason their connection count alarms.
//
/*
A source that will not open is skipped and named, not fatal.

It was fatal, on the reasoning that a server starting with three of its four
warehouses unreachable serves three-quarters of its reports and fails the rest
at six in the morning. The reasoning describes a failure that cannot happen
here: sql.Open does not connect, so an unreachable warehouse opens perfectly
well and fails at query time. What actually reaches this is a driver the build
cannot open and a ${secret:…} the deployment has not set — both configuration,
neither transient.

And both are publishable. `driver: mysql` was accepted by definition.Validate
long before any MySQL driver was registered, so an editor could publish one, get
a 200, and take the deployment down at its next restart — with the API down, so
the only way to remove it was a prompt on the database. That is the third time
the same shape has appeared, after a schedule's timezone and its cron.

So the source is skipped, its reason is returned for the caller to log and
count, and every report that does not read it keeps working. One that does read
it fails with a message naming the source, which is where somebody can act.
*/
func New(defs []definition.DataSource, secrets secret.Resolver, log *slog.Logger) (*Registry, error) {
	r := &Registry{
		sources:     map[string]*source{},
		federations: map[string]*federation{},
		log:         log,
		secrets:     secrets,
		unopened:    map[string]error{},
	}

	for _, def := range defs {
		s, err := r.prepare(def)
		if err != nil {
			r.unopened[def.Name] = err
			continue
		}
		r.sources[def.Name] = s
		if s.db != nil {
			log.Info("datasource", "name", def.Name, "driver", def.Driver,
				"timeout", def.Limits.Timeout(), "maxRows", def.Limits.Rows())
		}
	}
	return r, nil
}

/*
prepare resolves a definition into something answerable, without touching the
registry.

Shared by New and Adopt so a source that arrives through the API is opened
exactly as one read at startup. Two copies of this is how a deployment ends up
with a secret resolved on one path and left as literal ${secret:…} text on the
other — the drift this package has already been bitten by twice, in a driver
check and in a timezone.
*/
func (r *Registry) prepare(def definition.DataSource) (*source, error) {
	if def.Federated() {
		// An object store is not connected to; it is read through an engine
		// that can address files. It is registered so a dataset can name it,
		// and resolving one is what needs federation.
		//
		// Its URI is resolved all the same: a bucket URL can carry a
		// reference, and one left as literal ${secret:…} text becomes a path
		// the reader looks for and does not find.
		uri, err := secret.Resolve(def.URI, r.secrets)
		if err != nil {
			return nil, fmt.Errorf("datasource %q: %w", def.Name, err)
		}
		def.URI = uri
		// And its credentials, for the same reason and with more at stake: a
		// key left as literal ${secret:…} text is sent to the object store as
		// the key, which is a 403 that reads like a rotated credential rather
		// than an unset one.
		creds, err := secret.Resolve(def.Credentials, r.secrets)
		if err != nil {
			return nil, fmt.Errorf("datasource %q: %w", def.Name, err)
		}
		def.Credentials = creds
		return &source{def: def}, nil
	}
	return open(def, r.secrets)
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
		/*
		   The name and the driver, and not the DSN in either form.

		   This printed def.DSN — the definition's own text rather than the
		   resolved one, on the reasoning that the unresolved version says
		   ${secret:…} where the password goes. True when a definition uses a
		   secret reference and false when somebody wrote the password inline,
		   which is allowed and is what a first deployment does. The error goes
		   to the startup log, so that is a credential in the log of every
		   instance that failed to start.

		   The driver name is what a person needs anyway: this fails when the
		   build cannot open that kind of database, and no DSN would have told
		   them that.

		   Redacted rather than merely omitted, because leaving it out of the
		   format string is only half of it: the driver puts it back. A build
		   with the duckdb driver registered answers `driver: duckdb` with
		   `Cannot open file "duckdb://user:password@host/db"`, and that is the
		   text this wraps.
		*/
		return nil, fmt.Errorf("datasource %q (driver %q): %w",
			def.Name, def.Driver, secret.Redact(err, dsn))
	}

	// The pool is bounded because somebody else operates this database. A
	// reporting tool that opens two hundred connections during a burst is a
	// reporting tool their DBA turns off — which this said before it was true:
	// each of these was applied only when a definition set it, so a source
	// that said nothing got database/sql's defaults, and the first of those is
	// unlimited.
	db.SetMaxOpenConns(def.Pool.Open())
	db.SetMaxIdleConns(def.Pool.Idle())
	db.SetConnMaxIdleTime(def.Pool.IdleFor())
	db.SetConnMaxLifetime(def.Pool.LifetimeOf())

	/*
	   An in-memory database lives exactly as long as a connection to it.

	   SQLite destroys a `mode=memory` database when its last connection closes,
	   and the pool above is built to close connections: a few minutes idle and
	   every one of them is reaped. The next query then opens a *new*, empty
	   database and the report fails with "no such table" — against a source
	   whose Test button still passes, because connecting is not the problem.

	   The demo warehouse is exactly this, so the symptom is somebody opening
	   the demo, reading a report, coming back after lunch and finding every
	   report broken with no error anybody caused. Which is what happened here,
	   between one screenshot and the next.

	   Pinning the pool is the whole fix: one connection that is never idle-timed
	   and never aged out keeps the database alive for the life of the process.
	   It costs one connection to a database that is in this process's own
	   memory, and it applies to nothing else — a file or a server survives an
	   idle pool perfectly well, and their operators are the reason the limits
	   above exist.
	*/
	if memoryDatabase(dsn) {
		db.SetConnMaxIdleTime(0)
		db.SetConnMaxLifetime(0)
		if def.Pool.Idle() < 1 {
			db.SetMaxIdleConns(1)
		}
	}
	return &source{def: def, db: db, dialect: dialect}, nil
}

// memoryDatabase reports whether a DSN names a database that exists only while
// something is connected to it.
//
// Matched on the parameter rather than on the driver: `mode=memory` is how
// SQLite spells it, `:memory:` is the older form, and a source that says
// neither is on disk or on a server either way.
func memoryDatabase(dsn string) bool {
	return strings.Contains(dsn, "mode=memory") || strings.Contains(dsn, ":memory:")
}

/*
Engine resolves how to compile and run queries for a dataset.

One connection when one database can answer, which is the common report and
the cheap path. A federation when the dataset joins across databases or reads
an object store — neither of which a single connection can do, and both of
which need the engine that mounts them.
*/
func (r *Registry) Engine(ctx context.Context, ds definition.Dataset) (run.Engine, error) {
	if len(ds.Sources) == 0 {
		return run.Engine{}, fmt.Errorf("%w: %q", ErrNoSources, ds.Name)
	}
	if len(ds.Sources) == 1 && !r.needsFederation(ds.Sources[0]) {
		return r.single(ds, ds.Sources[0])
	}
	return r.federate(ctx, ds)
}

// needsFederation reports whether a lone source still needs an engine that can
// mount it — an object store holds files rather than a catalogue, so there is
// nothing to connect to.
func (r *Registry) needsFederation(ref definition.SourceRef) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.sources[ref.Ref]
	// An unknown source is not federated, so single reports it by name rather
	// than as a federation that could not mount something nobody registered.
	return ok && s.def.Federated()
}

func (r *Registry) single(ds definition.Dataset, ref definition.SourceRef) (run.Engine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.sources[ref.Ref]
	if !ok {
		return run.Engine{}, fmt.Errorf("%w: dataset %q reads %q", ErrUnknownSource, ds.Name, ref.Ref)
	}
	if s.db == nil {
		// Engine routes federated sources away before reaching this, so a nil
		// connection here means a kind of source that was registered without
		// one and without anything knowing it needs mounting. An error beats
		// an executor over a nil database, which fails at the first query with
		// the driver's words rather than with the source's name.
		return run.Engine{}, fmt.Errorf("%w: %q has no connection and was not federated",
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
	if err := r.closeFederations(); err != nil && first == nil {
		first = err
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
		/*
		   A source that is defined and would not open answers with the
		   reason, not with "no such datasource".

		   The two are opposite problems and the Test button showed the same
		   sentence for both: one means a name nobody has defined — a typo in
		   a dataset — and the other means the definition is right here and
		   this build cannot open it, which is a driver or a secret and is
		   acted on somewhere else entirely.
		*/
		r.mu.RLock()
		why := r.unopened[name]
		r.mu.RUnlock()
		if why != nil {
			return 0, why
		}
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

/*
Pool reports how busy one source's connections are.

From database/sql's own counters, which is the only party that knows without
asking the warehouse. Asking the warehouse means a query against a database
somebody else operates on every scrape — and the load harness did exactly that,
spawning a psql per sample, until the sampling cost enough to move the numbers
being sampled.

Three numbers, because any two of them say nothing. `open` is how many
connections exist, `inUse` is how many are running a query at this instant, and
`limit` is what the definition allows. In-use pinned at the limit while
throughput flattens is the pool being the ceiling; in-use well below it means
the ceiling is somewhere else and raising maxOpen only opens more connections on
somebody's production database.

A source with no pool — an object store, which is files rather than a database —
reports nothing rather than zeroes, because zero connections and no connections
are different answers.
*/
func (r *Registry) Pool(name string) (open, inUse, limit int, ok bool) {
	db, found := r.DB(name)
	if !found || db == nil {
		return 0, 0, 0, false
	}
	stats := db.Stats()
	return stats.OpenConnections, stats.InUse, stats.MaxOpenConnections, true
}
