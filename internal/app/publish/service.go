package publish

import (
	"context"
	"errors"
	"fmt"
	"strings"

	codec "github.com/gsoultan/cronos/internal/adapter/codec/yaml"
	"github.com/gsoultan/cronos/internal/app/run"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
	"github.com/gsoultan/cronos/internal/core/query"
)

// Datasets resolves a dataset the document under review refers to.
//
// Needed because a report cannot be checked alone: whether a filter binds to a
// real field is a question about the dataset, and the answer is the difference
// between a report that renders and one that fails on first open.
type Datasets interface {
	Dataset(ctx context.Context, name string) (definition.Dataset, error)
}

// Reports resolves a report a schedule names.
type Reports interface {
	Report(ctx context.Context, name string) (definition.Report, error)
}

// Live is the running view of the definitions, when storing is not enough to
// change it.
//
// A file-backed store rewrites the file and reloads the directory it was read
// from, so it needs nothing here. A database-backed one has no file to
// rewrite: without this, a published definition would be in the store and in
// the catalogue while every run kept using what the process read at startup.
type Live interface {
	Apply(raw []byte) error
}

/*
Engines resolves how a dataset's queries are compiled and run.

Given to publish so a definition can be proved against the database it will
actually read, rather than only against the dialect this package compiles for.
Compiling catches a field the dataset does not declare; only the database
catches a column it does not have, a permission the connection lacks, or a date
grain over a column stored as text — which works on SQLite and MySQL and is a
type error on Postgres.
*/
type Engines interface {
	Engine(ctx context.Context, ds definition.Dataset) (run.Engine, error)
}

// Verifier is an executor that can prove a statement without running it.
//
// Optional, and asserted for rather than required: an executor that cannot
// prepare — a federated one, a future one over an HTTP API — should not stop
// a definition being published.
type Verifier interface {
	Verify(ctx context.Context, p query.Plan) error
}

// Service validates and stores definitions.
type Service struct {
	store    Store
	datasets Datasets
	reports  Reports
	live     Live
	catalog  Catalog
	engines  Engines
}

// New wires a Service. The repository satisfies both lookups, so callers pass
// it once.
func New(s Store, d Datasets) *Service { return &Service{store: s, datasets: d} }

/*
WithEngines proves each block against the database it will read.

Publishing compiled every block and stopped there, which catches a field the
dataset does not declare and nothing the warehouse knows. The rest — a column
that is not there, a permission that is missing, a date grain over text —
arrived at six in the morning in the middle of a burst, in the driver's words.

A prepare, not a run: it parses, resolves every name and resolves every type,
touches no rows and holds no lock.
*/
func (s *Service) WithEngines(e Engines) *Service {
	s.engines = e
	return s
}

// WithLive makes a publish take effect without a restart.
func (s *Service) WithLive(l Live) *Service {
	s.live = l
	return s
}

// WithReports lets schedules be checked against the reports they run.
func (s *Service) WithReports(r Reports) *Service {
	s.reports = r
	return s
}

// Publish validates raw and stores it, whatever is there now.
func (s *Service) Publish(ctx context.Context, raw []byte, pr principal.Principal) (Result, error) {
	return s.PublishIf(ctx, raw, pr, "")
}

/*
PublishIf stores raw only if the definition is still at the version the caller
read.

Two people editing one report is not exotic — it is a Monday — and until now the
second save silently discarded the first. The version history means the lost
work is recoverable, which sounds like a mitigation and is not: nobody knows to
look, because nothing said anything. The author sees their change land and the
other author's disappear the next time they open the page.

An empty expectation stores unconditionally, which is deliberate and is what
every existing caller does. A deployment pipeline publishing from a git
repository *is* the source of truth and must not be refused because the running
copy differs — that difference is the point of the deploy. It is the editor,
which read a specific version and is proposing a change to it, that has
something to be stale about.

The window this closes is the one that matters and not the only one. Two saves
milliseconds apart can still both read the same current version and both
succeed; two people editing for ten minutes cannot. Closing the smaller window
means a conditional write in each store, which is worth doing when anybody has
ever hit it — and nobody has, because it needs two people to press a button in
the same instant on the same definition.
*/
func (s *Service) PublishIf(ctx context.Context, raw []byte, pr principal.Principal,
	expect string) (Result, error) {

	if !pr.CanEdit() {
		return Result{}, fmt.Errorf("%w: %s may not change definitions", ErrForbidden, pr.ProjectRole)
	}

	kind, err := codec.Loader{}.Kind(raw)
	if err != nil {
		return Result{}, err
	}

	name, err := s.check(ctx, kind, raw)
	if err != nil {
		return Result{}, err
	}

	if expect != "" {
		if err := s.unchanged(ctx, pr, kind, name, expect); err != nil {
			return Result{}, err
		}
	}

	version, err := s.store.Put(ctx, pr, kind, name, raw)
	if err != nil {
		return Result{}, err
	}

	// After the store, not before: a document that failed to store must not be
	// the one the next request runs. It has already been decoded twice by now,
	// so a failure here is not a bad document — it is the running view being
	// unable to hold a document the store accepted, and reporting success
	// would leave the two disagreeing with nobody told.
	if s.live != nil {
		if err := s.live.Apply(raw); err != nil {
			return Result{}, fmt.Errorf("publish: stored, but not live: %w", err)
		}
	}
	return Result{Kind: kind, Name: name, Version: version}, nil
}

// check proves the document will work, and returns the name to store it under.
//
// The name comes from the document rather than from the request path. A
// mismatch between the two is a rename someone did not mean, and taking the
// path would silently store one definition under another's name.
func (s *Service) check(ctx context.Context, kind string, raw []byte) (string, error) {
	switch kind {
	case codec.KindDataSource:
		/*
		   A datasource, which this switch did not mention until now.

		   The portal has had a four-step wizard for connecting one since before
		   there was a server to publish to, and every Save it ever made was
		   answered "unsupported kind" — a 400 from a screen that had just told
		   somebody their connection test passed. The codec has the kind, the
		   loader parses one, the file store keeps them; only this was missing.

		   The loader runs definition.Validate, which is what checks the driver
		   is one this build can open and that a connection string is present.
		   Deliberately no attempt to connect: that is what the test endpoint is
		   for, and a publish that fails because a warehouse is down at four in
		   the afternoon is a definition somebody cannot save for a reason that
		   has nothing to do with it.
		*/
		src, err := codec.Loader{}.DataSource(raw)
		if err != nil {
			return "", err
		}
		return src.Name, nil

	case codec.KindDataset:
		// The loader already ran definition.Validate.
		ds, err := codec.Loader{}.Dataset(raw)
		if err != nil {
			return "", err
		}
		if err := query.Check(ds); err != nil {
			return "", err
		}
		return ds.Name, nil

	case codec.KindReport:
		rep, err := codec.Loader{}.Report(raw)
		if err != nil {
			return "", err
		}
		return rep.Name, s.checkReport(ctx, rep)

	case codec.KindSchedule:
		sc, err := codec.Loader{}.Schedule(raw)
		if err != nil {
			return "", err
		}
		return sc.Name, s.checkSchedule(ctx, sc)
	}
	return "", fmt.Errorf("%w: %s", ErrUnsupported, kind)
}

// checkSchedule proves the schedule can run before it is allowed to.
//
// The check that earns its place is row scope. docs/tenancy.md sets out the
// rule and then says it is easy to get wrong, which is exactly what this
// turns from a paragraph into an error: a burst executes as the schedule's
// owner — a project member with no embed token — so a dataset carrying a
// .scope predicate matches nothing, and the burst delivers five thousand empty
// documents while reporting complete success. Nothing downstream can tell that
// apart from a month in which nobody was billed.
func (s *Service) checkSchedule(ctx context.Context, sc definition.Schedule) error {
	rep, err := s.reportNamed(ctx, sc.Report)
	if err != nil {
		return fmt.Errorf("%w: schedule %q runs report %q: %v",
			ErrNotFound, sc.Name, sc.Report, err)
	}
	if _, ok := rep.Output(sc.Output); !ok {
		return fmt.Errorf("%w: schedule %q renders output %q, which report %q does not have",
			ErrNotFound, sc.Name, sc.Output, sc.Report)
	}

	names := rep.Datasets()
	if sc.Bursts() {
		names = append(names, sc.Burst.Over.Dataset)
	}
	for _, name := range names {
		ds, err := s.datasets.Dataset(ctx, name)
		if err != nil {
			return fmt.Errorf("%w: schedule %q reads dataset %q: %v", ErrNotFound, sc.Name, name, err)
		}
		if len(ds.RowLevelSecurity) > 0 {
			return fmt.Errorf(
				"%w: schedule %q reads dataset %q, which has row-level security. "+
					"A burst runs as the schedule's owner and has no embed token, so the "+
					"predicate matches nothing and every document comes out empty. "+
					"Scope it with a parameter the schedule binds instead — docs/tenancy.md",
				ErrScopedBySchedule, sc.Name, name)
		}
	}
	return nil
}

// reportNamed resolves a report, if the service was given somewhere to look.
func (s *Service) reportNamed(ctx context.Context, name string) (definition.Report, error) {
	if s.reports == nil {
		return definition.Report{}, fmt.Errorf("no report repository configured")
	}
	return s.reports.Report(ctx, name)
}

// checkReport resolves everything the report reads and checks it against them.
func (s *Service) checkReport(ctx context.Context, rep definition.Report) error {
	sets := map[string]definition.Dataset{}
	for _, name := range rep.Datasets() {
		ds, err := s.datasets.Dataset(ctx, name)
		if err != nil {
			// A report naming a dataset nobody has is a report that renders
			// nothing, and it is far cheaper to say so now than to let someone
			// discover it when a customer opens the page.
			return fmt.Errorf("%w: report %q reads dataset %q: %v",
				ErrNotFound, rep.Name, name, err)
		}
		sets[name] = ds
	}
	// Filters are checked against every dataset that exists, not only the ones
	// this report reads. A filter may bind a dataset no block here uses — the
	// viewer announces such a filter as not applying, which is deliberate and
	// is what lets one filter definition serve a report that later gains a
	// block reading it. What is not deliberate is a dataset name nobody has,
	// and that stays an error.
	known := sets
	if s.catalog != nil {
		known = map[string]definition.Dataset{}
		for _, ds := range s.catalog.Datasets() {
			known[ds.Name] = ds
		}
		for name, ds := range sets {
			known[name] = ds
		}
	}
	if err := query.CheckFilters(rep.Filters, known); err != nil {
		return err
	}
	return s.checkBlocks(ctx, rep, sets)
}

// checkBlocks compiles every block, which is the only way to know they will.
//
// Compiling is cheaper than being wrong: it catches a field the dataset does
// not publish, an aggregate nobody implements, and a grain the dialect cannot
// express — all of which are otherwise found by whoever opens the report.
func (s *Service) checkBlocks(ctx context.Context, rep definition.Report,
	sets map[string]definition.Dataset) error {
	// Postgres, because it supports every grain: a check that used a narrower
	// dialect would pass definitions the eventual database cannot run, and one
	// that used a narrower one still would reject definitions that are fine.
	builder := query.NewBuilder(query.Postgres{})
	filters := query.Filters{Defs: rep.Filters}

	for _, out := range rep.Outputs {
		for i, blk := range out.Layout {
			if blk.Kind == definition.TextBlock {
				continue
			}
			ds := sets[blk.DatasetFor(rep.Dataset)]
			if _, _, err := builder.BuildBlock(ds, blk, defaults(ds), filters, checker(ds)); err != nil {
				return fmt.Errorf("%w: output %q block %d: %v", query.ErrBadTemplate, out.Name, i, err)
			}
			if err := s.provable(ctx, rep, out.Name, i, ds, blk, filters); err != nil {
				return err
			}
		}
	}
	return nil
}

/*
provable asks the database whether it would accept the block.

Compiled against the dialect the source actually speaks, not the one used
above: this statement is going to that database, and a plan built for Postgres
and prepared against MySQL would fail for the wrong reason.

Every failure here is the definition's, which is why the message is returned
whole. A warehouse that is unreachable is not the definition's fault and is not
a reason to refuse a publish — a deployment whose database is down should still
be able to fix the report that is waiting for it.
*/
func (s *Service) provable(ctx context.Context, rep definition.Report, output string, i int,
	ds definition.Dataset, blk definition.Block, filters query.Filters) error {

	if s.engines == nil {
		return nil
	}
	/*
	   The caller's context, which this used to discard for a fresh Background.

	   Every other step of a publish is cancellable and this one is not, which
	   is backwards: it is the only step that talks to a database somebody else
	   operates. A report with four outputs of six blocks prepares twenty-four
	   statements against their warehouse, and a browser tab closed halfway
	   through left all of them to run to their own timeout with nobody waiting
	   for the answer.

	   The statement timeout below still bounds each prepare, so this was never
	   unbounded — it was work continuing after the only party who wanted it had
	   gone, which on a warehouse with a connection limit is somebody else's
	   incident.
	*/
	engine, err := s.engines.Engine(ctx, ds)
	if err != nil {
		return nil // no engine for it here; the render will say so
	}
	verifier, ok := engine.Executor.(Verifier)
	if !ok {
		return nil
	}

	plan, _, err := engine.Builder.BuildBlock(ds, blk, defaults(ds), filters, checker(ds))
	if err != nil {
		return fmt.Errorf("%w: output %q block %d: %v", query.ErrBadTemplate, output, i, err)
	}
	if err := verifier.Verify(ctx, plan); err != nil {
		if unreachable(err) {
			return nil
		}
		return fmt.Errorf("%w: output %q block %d: the database refused this query: %v%s",
			query.ErrBadTemplate, output, i, err, hint(err, ds, blk))
	}
	return nil
}

/*
hint adds what to do about it, for the failures worth recognising.

One so far, and it is the one every warehouse eventually produces: a column
holding dates as text. SQLite is typeless and MySQL coerces, so a dataset that
works in development fails on the Postgres it was written for — with a message
from the driver that names a function signature and not the column.

Narrow on purpose. A hint that guesses is worse than none: somebody follows it,
it does not help, and the real message was there all along.
*/
func hint(err error, ds definition.Dataset, blk definition.Block) string {
	text := strings.ToLower(err.Error())
	if !strings.Contains(text, "date_trunc") || !strings.Contains(text, "text") {
		return ""
	}

	field := blk.X.Field
	if field == "" {
		return ""
	}
	declared, ok := ds.Field(field)
	if !ok || declared.Type != "date" {
		return ""
	}
	return fmt.Sprintf(
		` — %q is declared as a date and this warehouse stores it as text. `+
			`Cast it in the dataset's query: CAST(%s AS date) AS %s`,
		field, field, field)
}

/*
unreachable reports whether the failure was the database being away rather than
the definition being wrong.

Matched on text, which is unlovely and is the only thing available: database/sql
returns driver errors unwrapped and every driver spells this differently. Wrong
in the safe direction — a definition wrongly published is fixed by publishing
again, and a publish wrongly refused during an outage is somebody unable to fix
the report that is waiting for the outage to end.
*/
func unreachable(err error) bool {
	text := strings.ToLower(err.Error())
	for _, sign := range []string{
		"connection refused", "no such host", "i/o timeout", "context deadline",
		"connection reset", "broken pipe", "server closed", "database is closed",
		"too many connections", "connection timed out",
	} {
		if strings.Contains(text, sign) {
			return true
		}
	}
	return false
}

// defaults supplies a value for every required parameter, so compilation is
// testing the block's shape rather than whether someone remembered to pass a
// date to a validator.
func defaults(ds definition.Dataset) map[string]any {
	in := map[string]any{}
	for _, p := range ds.Params {
		if p.Required && !p.HasDefault() {
			in[p.Name] = placeholder(p)
		}
	}
	return in
}

func placeholder(p definition.Param) any {
	switch p.Type {
	case definition.Date:
		return "2000-01-01"
	case definition.Number:
		return float64(0)
	case definition.Bool:
		return false
	case definition.Enum:
		if len(p.Values) > 0 {
			if p.Multiple {
				return []any{p.Values[0]}
			}
			return p.Values[0]
		}
	}
	if p.Multiple {
		return []any{""}
	}
	return ""
}

// checker is a principal that satisfies every row scope the dataset declares,
// so compilation exercises the real predicate rather than the FALSE a
// scope-less caller would get.
func checker(ds definition.Dataset) principal.Principal {
	scope := map[string]string{}
	for _, s := range ds.RowLevelSecurity {
		for _, name := range query.ScopeKeys(s.Predicate) {
			scope[name] = "check"
		}
	}
	return principal.Principal{Subject: "publish-check", ProjectRole: principal.ProjectViewer, Scope: scope}
}

/*
unchanged refuses a save built on a version somebody has already replaced.

Reads what is stored and compares its content address. The alternative — asking
the store to compare — would be a conditional write and a wider port; this is
one Get against a definition, on a path that already does several.

A definition that has been deleted is a conflict too, and a pointed one: the
caller is editing something that no longer exists, and storing theirs would
bring it back without anybody deciding to.
*/
func (s *Service) unchanged(ctx context.Context, pr principal.Principal,
	kind, name, expect string) error {

	current, err := s.store.Get(ctx, pr, kind, name)
	switch {
	case errors.Is(err, ErrNotFound):
		return fmt.Errorf("%w: %s %q was deleted while you were editing it",
			ErrStale, kind, name)
	case err != nil:
		return err
	}

	if got := Version(current); got != expect {
		return fmt.Errorf(
			"%w: %s %q is at %s and you started from %s — somebody else saved it",
			ErrStale, kind, name, got, expect)
	}
	return nil
}
