package query

import (
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// A report whose blocks read two datasets. Period means a different field in
// each, and Status means nothing at all in shipments.
func sharedFilters() []definition.Filter {
	return []definition.Filter{
		{Name: "period", Label: "Period", Type: definition.Date,
			Bind: map[string]string{"invoices": "issued_at", "shipments": "shipped_at"}},
		{Name: "status", Label: "Status", Type: definition.Enum,
			Bind: map[string]string{"invoices": "status"}},
	}
}

func withField(ds definition.Dataset, names ...string) definition.Dataset {
	for _, n := range names {
		ds.Fields = append(ds.Fields, definition.Field{
			Name: n, Type: "string", Role: definition.Dimension})
	}
	return ds
}

// where returns only what the wrapper added. The dataset's own query mentions
// issued_at and defaults status to "sent", so asserting against the whole
// statement would pass on the fixture rather than on the filter.
func whereOf(sql string) string {
	_, after, found := strings.Cut(sql, "AS "+subqueryAlias)
	if !found {
		return ""
	}
	return after
}

func buildWith(t *testing.T, ds definition.Dataset, f Filters) (Plan, Coverage) {
	t.Helper()
	plan, cov, err := NewBuilder(Postgres{}).WithClock(jan1).BuildWith(ds,
		map[string]any{"from": "2026-07-01"}, f, embedded("c-9"))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return plan, cov
}

func TestAFilterBindsToTheDatasetsOwnField(t *testing.T) {
	ds := withField(invoices(), "issued_at", "status")
	plan, cov := buildWith(t, ds, Filters{
		Defs: sharedFilters(),
		Values: map[string]FilterValue{
			"period": {Op: Between, Values: []any{"2026-07-01", "2026-07-31"}},
			"status": {Op: In, Values: []any{"sent", "overdue"}},
		},
	})

	if !strings.Contains(plan.SQL(), "issued_at BETWEEN") {
		t.Errorf("period did not bind to issued_at:\n%s", plan.SQL())
	}
	if !strings.Contains(plan.SQL(), "status IN (") {
		t.Errorf("status did not compile to IN:\n%s", plan.SQL())
	}
	if !cov.Complete() {
		t.Errorf("both filters bind to invoices, ignored = %v", cov.Ignored)
	}
	if !contains(plan.Args(), "overdue") {
		t.Errorf("filter values must be bound, args = %v", plan.Args())
	}
}

// The same filter, a different dataset, a different field. This is the whole
// reason bind is a map and not a field name.
func TestTheSameFilterBindsDifferentlyPerDataset(t *testing.T) {
	shipments := withField(invoices(), "shipped_at")
	shipments.Name = "shipments"
	shipments.RowLevelSecurity = nil

	plan, cov := buildWith(t, shipments, Filters{
		Defs:   sharedFilters(),
		Values: map[string]FilterValue{"period": {Op: Gte, Values: []any{"2026-07-01"}}},
	})

	if !strings.Contains(plan.SQL(), "shipped_at >=") {
		t.Errorf("period should narrow shipped_at here:\n%s", plan.SQL())
	}
	if strings.Contains(whereOf(plan.SQL()), "issued_at") {
		t.Errorf("the invoices binding leaked into shipments:\n%s", plan.SQL())
	}
	// The report format's promise: the block says so rather than leaving it to
	// be discovered.
	if want := []string{"status"}; strings.Join(cov.Ignored, ",") != strings.Join(want, ",") {
		t.Errorf("ignored = %v, want %v", cov.Ignored, want)
	}
	if cov.Complete() {
		t.Error("a block missing a binding is not fully covered")
	}
}

// A filter with no binding for this dataset must not narrow it, and must not
// be silently forgotten either.
func TestAnUnboundFilterChangesNothing(t *testing.T) {
	shipments := withField(invoices(), "shipped_at")
	shipments.Name = "shipments"
	shipments.RowLevelSecurity = nil

	plain, _ := buildWith(t, shipments, Filters{})
	filtered, cov := buildWith(t, shipments, Filters{
		Defs: sharedFilters(),
		// A value the dataset's own defaults never produce, so finding it in
		// the arguments can only mean the unbound filter bound it.
		Values: map[string]FilterValue{"status": {Op: Eq, Values: []any{"written-off"}}},
	})

	if plain.SQL() != filtered.SQL() {
		t.Errorf("an unbound filter changed the statement:\n%s\n---\n%s", plain.SQL(), filtered.SQL())
	}
	if contains(filtered.Args(), "written-off") {
		t.Errorf("an unbound filter bound its value anyway: %v", filtered.Args())
	}
	if !strings.Contains(strings.Join(cov.Ignored, ","), "status") {
		t.Errorf("the block should report status as not applying, got %v", cov.Ignored)
	}
}

// Filters narrow inside the same wrapper as row scope, so scope is never
// widened by one and the placeholders stay in text order.
func TestFiltersNarrowAlongsideRowScope(t *testing.T) {
	ds := withField(invoices(), "issued_at")
	plan, _ := buildWith(t, ds, Filters{
		Defs:   sharedFilters()[:1],
		Values: map[string]FilterValue{"period": {Op: Gte, Values: []any{"2026-07-01"}}},
	})

	scopeAt := strings.Index(plan.SQL(), "customer_id = $")
	filterAt := strings.Index(plan.SQL(), "issued_at >= $")
	if scopeAt < 0 || filterAt < 0 {
		t.Fatalf("both predicates should be present:\n%s", plan.SQL())
	}
	if scopeAt > filterAt {
		t.Errorf("row scope should be written first:\n%s", plan.SQL())
	}
	// Scope is $4, so the filter must be $5 — text order is argument order.
	if !strings.Contains(plan.SQL(), "issued_at >= $5") {
		t.Errorf("placeholder numbering does not follow the text:\n%s", plan.SQL())
	}
	if plan.Args()[4] != "2026-07-01" {
		t.Errorf("argument 5 is %v, want the filter value", plan.Args()[4])
	}
}

// A declared filter left blank still covers the block. Reporting it as ignored
// would say the block cannot be filtered at all, which is a different claim.
func TestABlankFilterStillCoversTheBlock(t *testing.T) {
	ds := withField(invoices(), "issued_at", "status")
	plan, cov := buildWith(t, ds, Filters{Defs: sharedFilters()})

	if !cov.Complete() {
		t.Errorf("both filters bind here even with no value, ignored = %v", cov.Ignored)
	}
	if strings.Contains(whereOf(plan.SQL()), "issued_at") {
		t.Errorf("a blank filter should not narrow anything:\n%s", plan.SQL())
	}
}

func TestFiltersAreRefused(t *testing.T) {
	ds := withField(invoices(), "issued_at")
	cases := []struct {
		name  string
		value FilterValue
		bind  map[string]string
		want  error
		says  string
	}{
		{"an operator nobody implements", FilterValue{Op: "regex", Values: []any{"^a"}},
			nil, ErrBadArgument, "not one of them"},
		{"between with one value", FilterValue{Op: Between, Values: []any{"2026-07-01"}},
			nil, ErrBadArgument, "takes 2 value(s)"},
		{"in with nothing to match", FilterValue{Op: In},
			nil, ErrBadArgument, "nothing to match"},
		// The field is the one part of the predicate that is text, so it must
		// name a field this dataset actually publishes.
		{"a binding to a field that does not exist", FilterValue{Op: Eq, Values: []any{"x"}},
			map[string]string{"invoices": "sneaky_column"}, ErrBadTemplate, "not a field of it"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			def := sharedFilters()[0]
			if c.bind != nil {
				def.Bind = c.bind
			}
			_, _, err := NewBuilder(Postgres{}).WithClock(jan1).BuildWith(ds,
				map[string]any{"from": "2026-07-01"},
				Filters{Defs: []definition.Filter{def},
					Values: map[string]FilterValue{"period": c.value}},
				embedded("c-9"))

			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("message %q does not say %q", err, c.says)
			}
		})
	}
}

// Someone searching for "50%" wants a discount, not every row.
func TestContainsEscapesWildcards(t *testing.T) {
	ds := withField(invoices(), "customer_ref")
	def := definition.Filter{Name: "ref", Type: definition.String,
		Bind: map[string]string{"invoices": "customer_ref"}}

	plan, _ := buildWith(t, ds, Filters{
		Defs:   []definition.Filter{def},
		Values: map[string]FilterValue{"ref": {Op: Contains, Values: []any{"50% a_b"}}},
	})

	if !strings.Contains(plan.SQL(), `LIKE $5 ESCAPE '\'`) {
		t.Errorf("contains should name its escape character:\n%s", plan.SQL())
	}
	if !contains(plan.Args(), `%50\% a\_b%`) {
		t.Errorf("wildcards in the search text were not escaped: %v", plan.Args())
	}
}

func TestBindsReportsAnEmptyMappingAsUnbound(t *testing.T) {
	f := definition.Filter{Name: "period", Bind: map[string]string{"invoices": ""}}
	if field, ok := f.Binds("invoices"); ok {
		t.Errorf("an empty binding is not a binding, got %q", field)
	}
}
