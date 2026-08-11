package query

import (
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/core/principal"
)

func mustFail(t *testing.T, ds definition.Dataset, in map[string]any, want error) error {
	t.Helper()
	_, err := NewBuilder(Dollar{}).WithClock(jan1).Build(ds, in, embedded("c-9"))
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
	return err
}

func TestArgumentsAreRefused(t *testing.T) {
	base := map[string]any{"from": "2026-07-01", "to": "2026-07-31"}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for key, val := range base {
			m[key] = val
		}
		m[k] = v
		return m
	}

	cases := []struct {
		name string
		in   map[string]any
		says string
	}{
		// Ignoring it would tell a caller their filter applied when it did not.
		{"an unknown parameter", with("custmer_id", "c-9"), "not a parameter"},
		{"a missing required one", map[string]any{"to": "2026-07-31"}, `"from" is required`},
		{"an enum value nobody declared", with("status", []any{"deleted"}), "not a value"},
		{"a date in the wrong order", with("from", "01/07/2026"), "YYYY-MM-DD"},
		{"a scalar where a list belongs", with("status", "sent"), "takes a list"},
		{"a number as text", with("from", 20260701), "YYYY-MM-DD"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := mustFail(t, invoices(), c.in, ErrBadArgument)
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("message %q does not say %q", err, c.says)
			}
		})
	}
}

// An error that quotes the input back is a probe oracle, and a reflected
// payload in whatever renders it.
func TestErrorsDoNotEchoTheValue(t *testing.T) {
	payload := "<script>alert(1)</script>"
	ds := invoices()
	ds.Params = append(ds.Params, definition.Param{Name: "amount", Type: definition.Number})

	err := mustFail(t, ds, map[string]any{
		"from": "2026-07-01", "to": "2026-07-31", "amount": payload,
	}, ErrBadArgument)

	if strings.Contains(err.Error(), payload) {
		t.Errorf("the error repeats the caller's input: %v", err)
	}
}

func TestTemplatesAreRefused(t *testing.T) {
	cases := []struct {
		name  string
		query string
		says  string
	}{
		// Binding nil here would return the wrong rows rather than fail.
		{"a hole naming no parameter", "SELECT 1 WHERE x = {{ .params.nope }}", "does not declare"},
		{"a hole reaching somewhere else", "SELECT 1 WHERE x = {{ .env.SECRET }}", "not .params.name"},
		{"an expression", "SELECT 1 WHERE x = {{ len .params.from }}", "not .params.name"},
		{"a bare dot", "SELECT 1 WHERE x = {{ . }}", "not .params.name"},
		{"an unclosed hole", "SELECT 1 WHERE x = {{ .params.from", "never closed"},
		// Only a predicate can fail closed, so scope may only appear in one.
		{"scope outside a predicate", "SELECT 1 WHERE x = {{ .scope.customer_id }}",
			"only available in rowLevelSecurity"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ds := invoices()
			ds.Query = c.query
			ds.RowLevelSecurity = nil
			err := mustFail(t, ds, map[string]any{"from": "2026-07-01"}, ErrBadTemplate)
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("message %q does not say %q", err, c.says)
			}
		})
	}
}

// A predicate is a template too, and a broken one must fail the build rather
// than compile to something that lets rows through.
func TestABrokenPredicateFailsTheBuild(t *testing.T) {
	ds := invoices()
	ds.RowLevelSecurity = []definition.RowScope{{Predicate: "customer_id = {{ .params.from"}}
	_, err := NewBuilder(Dollar{}).WithClock(jan1).
		Build(ds, map[string]any{"from": "2026-07-01"}, embedded("c-9"))
	if !errors.Is(err, ErrBadTemplate) {
		t.Fatalf("got %v, want ErrBadTemplate", err)
	}
}

// Scope keys the dataset never asks for cannot widen anything — the predicate
// decides which keys matter, not the token.
func TestExtraScopeClaimsAreInert(t *testing.T) {
	pr := principal.Principal{Subject: "u1",
		Scope: map[string]string{"customer_id": "c-9", "is_admin": "true", "org_id": "*"}}

	plan, err := NewBuilder(Dollar{}).WithClock(jan1).
		Build(invoices(), map[string]any{"from": "2026-07-01"}, pr)
	if err != nil {
		t.Fatal(err)
	}
	if contains(plan.Args(), "true") || contains(plan.Args(), "*") {
		t.Errorf("an unrequested claim was bound: %v", plan.Args())
	}
}
