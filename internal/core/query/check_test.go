package query

import (
	"errors"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

func TestCheckAcceptsAWorkingDataset(t *testing.T) {
	if err := Check(invoices()); err != nil {
		t.Errorf("the reference dataset should pass: %v", err)
	}
}

func TestCheckRejects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*definition.Dataset)
		says string
	}{
		{"scope in the query, where it cannot fail closed", func(d *definition.Dataset) {
			d.Query += " AND customer_id = {{ .scope.customer_id }}"
		}, "matches no rows instead of matching everything"},

		{"a hole for a parameter nobody declared", func(d *definition.Dataset) {
			d.Query += " AND region = {{ .params.region }}"
		}, "does not declare"},

		{"a predicate that reads no scope", func(d *definition.Dataset) {
			d.RowLevelSecurity = []definition.RowScope{{Predicate: "status <> 'draft'"}}
		}, "restricts every caller identically"},

		{"a predicate that will not parse", func(d *definition.Dataset) {
			d.RowLevelSecurity = []definition.RowScope{{Predicate: "x = {{ .scope.a"}}
		}, "never closed"},

		{"a predicate using an undeclared parameter", func(d *definition.Dataset) {
			d.RowLevelSecurity = []definition.RowScope{
				{Predicate: "customer_id = {{ .scope.customer_id }} AND r = {{ .params.region }}"}}
		}, "does not declare"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ds := invoices()
			c.edit(&ds)
			err := Check(ds)
			if !errors.Is(err, ErrBadTemplate) {
				t.Fatalf("got %v, want ErrBadTemplate", err)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("message %q does not say %q", err, c.says)
			}
		})
	}
}
