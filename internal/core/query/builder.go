package query

import (
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

// Builder compiles datasets into plans.
//
// It is the one place parameter binding happens, which is what makes binding
// reviewable: there is a single function to read in order to know that no
// caller value has ever been concatenated into SQL.
type Builder struct {
	dialect Dialect
	now     func() time.Time
}

// NewBuilder returns a Builder compiling for d.
func NewBuilder(d Dialect) Builder {
	return Builder{dialect: d, now: time.Now}
}

// WithClock returns a copy that resolves relative dates against now. Tests use
// it; so will a scheduled run, which must resolve "today" against the run's
// own instant and not against whenever the worker got to it.
func (b Builder) WithClock(now func() time.Time) Builder {
	b.now = now
	return b
}

// Build compiles ds into a plan for pr, with in as the caller's arguments.
//
// The dataset is assumed to have passed definition.Validate on save. Build
// re-checks only what it must in order to be safe with this particular call —
// that every hole names a declared parameter, and that every value binds —
// because it runs per query and validation runs per edit.
func (b Builder) Build(ds definition.Dataset, in map[string]any, pr principal.Principal) (Plan, error) {
	plan, _, err := b.BuildWith(ds, in, Filters{}, pr)
	return plan, err
}

// BuildWith compiles ds with a report's shared filters applied, and reports
// which of them reached this dataset.
//
// The Coverage is not diagnostic output. A block whose dataset has no binding
// for a filter is unaffected by it, and the report format promises the
// interface will say so on the block — which it can only do if compilation
// tells it. Returning it beside the plan makes that hard to forget.
func (b Builder) BuildWith(ds definition.Dataset, in map[string]any, f Filters,
	pr principal.Principal) (Plan, Coverage, error) {

	params, err := resolve(ds, in, b.now)
	if err != nil {
		return Plan{}, Coverage{}, err
	}

	bd := &binder{
		ph:       b.dialect,
		params:   params,
		scope:    pr.Scope,
		declared: declaredNames(ds),
	}

	inner, err := bd.render(ds.Query, false)
	if err != nil {
		return Plan{}, Coverage{}, err
	}

	// Row scope first, then the caller's filters. Both narrow, so the order
	// does not change the result — but it fixes the placeholder numbering, and
	// two blocks on one dataset must compile to the same statement or they
	// cannot share a cache entry.
	preds, err := bd.scopePredicates(ds.RowLevelSecurity)
	if err != nil {
		return Plan{}, Coverage{}, err
	}
	filtered, cov, err := bd.predicates(ds, f)
	if err != nil {
		return Plan{}, Coverage{}, err
	}

	return Plan{sql: bd.wrap(inner, append(preds, filtered...)), args: bd.args}, cov, nil
}

func declaredNames(ds definition.Dataset) map[string]bool {
	names := make(map[string]bool, len(ds.Params))
	for _, p := range ds.Params {
		names[p.Name] = true
	}
	return names
}
