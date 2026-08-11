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
	ph  Placeholder
	now func() time.Time
}

// NewBuilder returns a Builder writing ph's placeholders.
func NewBuilder(ph Placeholder) Builder {
	return Builder{ph: ph, now: time.Now}
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
	params, err := resolve(ds, in, b.now)
	if err != nil {
		return Plan{}, err
	}

	bd := &binder{
		ph:       b.ph,
		params:   params,
		scope:    pr.Scope,
		declared: declaredNames(ds),
	}

	inner, err := bd.render(ds.Query, false)
	if err != nil {
		return Plan{}, err
	}
	sql, err := bd.wrap(inner, ds.RowLevelSecurity)
	if err != nil {
		return Plan{}, err
	}
	return Plan{sql: sql, args: bd.args}, nil
}

func declaredNames(ds definition.Dataset) map[string]bool {
	names := make(map[string]bool, len(ds.Params))
	for _, p := range ds.Params {
		names[p.Name] = true
	}
	return names
}
