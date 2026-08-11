package query

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

func invoices() definition.Dataset {
	return definition.Dataset{
		Name:    "invoices",
		Sources: []definition.SourceRef{{Ref: "warehouse"}},
		Query: `SELECT i.id, c.name AS customer_name, i.total
FROM warehouse.invoices i
JOIN warehouse.customers c ON c.id = i.customer_id
WHERE i.issued_at BETWEEN {{ .params.from }} AND {{ .params.to }}
  AND i.status = ANY({{ .params.status }});`,
		Params: []definition.Param{
			{Name: "from", Type: definition.Date, Required: true},
			{Name: "to", Type: definition.Date, Required: true, Default: "today"},
			{Name: "status", Type: definition.Enum, Multiple: true,
				Values: []string{"draft", "sent", "paid", "overdue"}, Default: []any{"sent"}},
		},
		Fields: []definition.Field{
			{Name: "id", Type: "string", Role: definition.Dimension},
			{Name: "customer_name", Type: "string", Role: definition.Dimension},
			{Name: "total", Type: "decimal", Role: definition.Measure, Aggregate: "sum"},
		},
		RowLevelSecurity: []definition.RowScope{
			{Predicate: "customer_id = {{ .scope.customer_id }}"},
		},
	}
}

var jan1 = func() time.Time { return time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC) }

func embedded(customer string) principal.Principal {
	return principal.Principal{
		Subject: "u1", OrgID: "o1", ProjectID: "p1",
		ProjectRole: principal.ProjectViewer,
		Scope:       map[string]string{"customer_id": customer},
	}
}

func build(t *testing.T, ds definition.Dataset, in map[string]any, pr principal.Principal) Plan {
	t.Helper()
	plan, err := NewBuilder(Dollar{}).WithClock(jan1).Build(ds, in, pr)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return plan
}

// The central claim of the package: a caller's value reaches the database as an
// argument and never as text. If this test can be made to fail, everything
// else here is decoration.
func TestValuesNeverReachTheSQL(t *testing.T) {
	nasty := "2026-07-01' OR '1'='1"
	_, err := NewBuilder(Dollar{}).WithClock(jan1).Build(invoices(),
		map[string]any{"from": nasty, "to": "2026-07-31"}, embedded("c-9"))
	if !errors.Is(err, ErrBadArgument) {
		t.Fatalf("a malformed date should be refused, got %v", err)
	}

	// And where the type does admit arbitrary text, it still only binds.
	ds := invoices()
	ds.Params = append(ds.Params, definition.Param{Name: "note", Type: definition.String})
	ds.Query += " AND note = {{ .params.note }}"
	plan := build(t, ds, map[string]any{
		"from": "2026-07-01", "to": "2026-07-31", "note": "'; DROP TABLE invoices; --",
	}, embedded("c-9"))

	if strings.Contains(plan.SQL(), "DROP TABLE") {
		t.Fatalf("caller text reached the statement:\n%s", plan.SQL())
	}
	if !contains(plan.Args(), "'; DROP TABLE invoices; --") {
		t.Errorf("the value should still be bound, args = %v", plan.Args())
	}
}

func TestParamsBecomePlaceholdersInOrder(t *testing.T) {
	plan := build(t, invoices(),
		map[string]any{"from": "2026-07-01", "to": "2026-07-31", "status": []any{"sent", "overdue"}},
		embedded("c-9"))

	for _, want := range []string{"$1", "$2", "$3", "$4"} {
		if !strings.Contains(plan.SQL(), want) {
			t.Errorf("statement is missing %s:\n%s", want, plan.SQL())
		}
	}
	if len(plan.Args()) != 4 {
		t.Fatalf("got %d args, want 4: %v", len(plan.Args()), plan.Args())
	}
	if plan.Args()[0] != "2026-07-01" || plan.Args()[1] != "2026-07-31" {
		t.Errorf("dates bound out of order: %v", plan.Args()[:2])
	}
	// The wrapper's arguments follow the inner query's, because the text does.
	if plan.Args()[3] != "c-9" {
		t.Errorf("scope value should be the last argument, got %v", plan.Args()[3])
	}
}

func TestRowScopeIsAlwaysApplied(t *testing.T) {
	plan := build(t, invoices(),
		map[string]any{"from": "2026-07-01", "to": "2026-07-31"}, embedded("c-9"))

	if !strings.Contains(plan.SQL(), "WHERE (customer_id = $4)") {
		t.Errorf("row scope is not in the statement:\n%s", plan.SQL())
	}
	if !strings.Contains(plan.SQL(), "AS "+subqueryAlias) {
		t.Errorf("the dataset query was not wrapped:\n%s", plan.SQL())
	}
}

// docs/tenancy.md: an absent scope value means the predicate matches nothing.
// Never dropped, never read as "no constraint".
func TestAbsentScopeMatchesNothing(t *testing.T) {
	member := principal.Principal{Subject: "u2", OrgID: "o1", ProjectID: "p1",
		ProjectRole: principal.ProjectEditor} // no embed token, so no scope

	plan := build(t, invoices(),
		map[string]any{"from": "2026-07-01", "to": "2026-07-31"}, member)

	if !strings.Contains(plan.SQL(), "WHERE ("+noRows+")") {
		t.Fatalf("a scope-less caller should match no rows, got:\n%s", plan.SQL())
	}
	if strings.Contains(plan.SQL(), "customer_id =") {
		t.Errorf("the predicate should be replaced, not rendered with an empty value:\n%s", plan.SQL())
	}
	if len(plan.Args()) != 3 {
		t.Errorf("nothing should be bound for a dropped predicate, args = %v", plan.Args())
	}
}

// A predicate author cannot write their way out of the missing-scope rule. The
// whole predicate is replaced, so what it would have done is irrelevant.
func TestAbsentScopeSurvivesADefensivePredicate(t *testing.T) {
	for _, pred := range []string{
		"COALESCE({{ .scope.customer_id }}, customer_id) = customer_id",
		"customer_id <> {{ .scope.customer_id }}",
		"customer_id NOT IN ({{ .scope.customer_id }})",
		"TRUE OR customer_id = {{ .scope.customer_id }}",
	} {
		ds := invoices()
		ds.RowLevelSecurity = []definition.RowScope{{Predicate: pred}}
		plan := build(t, ds, map[string]any{"from": "2026-07-01", "to": "2026-07-31"},
			principal.Principal{Subject: "u2"})

		if !strings.HasSuffix(plan.SQL(), "WHERE ("+noRows+")") {
			t.Errorf("predicate %q was not replaced:\n%s", pred, plan.SQL())
		}
	}
}

func TestScopeIsBoundNotInterpolated(t *testing.T) {
	plan := build(t, invoices(), map[string]any{"from": "2026-07-01", "to": "2026-07-31"},
		embedded("c-9' OR '1'='1"))

	if strings.Contains(plan.SQL(), "OR '1'='1") {
		t.Fatalf("a scope claim reached the statement:\n%s", plan.SQL())
	}
	if !contains(plan.Args(), "c-9' OR '1'='1") {
		t.Errorf("the scope value should be bound, args = %v", plan.Args())
	}
}

func TestNoRowScopeMeansNoWrapper(t *testing.T) {
	ds := invoices()
	ds.RowLevelSecurity = nil
	plan := build(t, ds, map[string]any{"from": "2026-07-01", "to": "2026-07-31"},
		principal.Principal{Subject: "u2"})

	if strings.Contains(plan.SQL(), subqueryAlias) {
		t.Errorf("a dataset with no row scope should not be wrapped:\n%s", plan.SQL())
	}
}

// An author's trailing semicolon is a statement terminator and a syntax error
// once the query is a subquery.
func TestTheTrailingSemicolonIsTrimmed(t *testing.T) {
	plan := build(t, invoices(), map[string]any{"from": "2026-07-01", "to": "2026-07-31"},
		embedded("c-9"))
	if strings.Contains(plan.SQL(), ";\n) AS") {
		t.Errorf("the subquery still terminates itself:\n%s", plan.SQL())
	}
}

func TestDefaultsAndTheClock(t *testing.T) {
	plan := build(t, invoices(), map[string]any{"from": "2026-07-01"}, embedded("c-9"))

	if plan.Args()[1] != "2026-07-15" {
		t.Errorf(`"today" resolved to %v, want the clock's 2026-07-15`, plan.Args()[1])
	}
	if !contains(plan.Args(), "sent") {
		t.Errorf("the status default was not applied: %v", plan.Args())
	}
}

func TestQuestionPlaceholders(t *testing.T) {
	plan, err := NewBuilder(Question{}).WithClock(jan1).
		Build(invoices(), map[string]any{"from": "2026-07-01"}, embedded("c-9"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.SQL(), "$1") {
		t.Errorf("MySQL does not number placeholders:\n%s", plan.SQL())
	}
	if n := strings.Count(plan.SQL(), "?"); n != 4 {
		t.Errorf("got %d placeholders, want 4:\n%s", n, plan.SQL())
	}
}

func TestZeroPlanIsEmpty(t *testing.T) {
	if !(Plan{}).Empty() {
		t.Error("the zero Plan must be refusable by an executor")
	}
}

func contains(args []any, want string) bool {
	for _, a := range args {
		if s, ok := a.(string); ok && s == want {
			return true
		}
		if list, ok := a.([]any); ok && contains(list, want) {
			return true
		}
	}
	return false
}
