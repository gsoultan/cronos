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

func TestCheckFiltersAcceptsTheDocumentedExample(t *testing.T) {
	// docs/report-format.md: one filter, two datasets, a different field in each.
	shipments := invoices()
	shipments.Name = "shipments"
	shipments.Fields = append(shipments.Fields,
		definition.Field{Name: "shipped_at", Type: "date", Role: definition.Dimension})
	inv := invoices()
	inv.Fields = append(inv.Fields,
		definition.Field{Name: "issued_at", Type: "date", Role: definition.Dimension})

	filters := []definition.Filter{{
		Name: "period", Label: "Period", Type: definition.Date,
		Bind: map[string]string{"invoices": "issued_at", "shipments": "shipped_at"},
	}}
	sets := map[string]definition.Dataset{"invoices": inv, "shipments": shipments}

	if err := CheckFilters(filters, sets); err != nil {
		t.Errorf("the format's own example should pass: %v", err)
	}
}

func TestCheckFiltersRejects(t *testing.T) {
	inv := invoices()
	inv.Fields = append(inv.Fields,
		definition.Field{Name: "issued_at", Type: "date", Role: definition.Dimension})
	sets := map[string]definition.Dataset{"invoices": inv}

	cases := []struct {
		name string
		f    definition.Filter
		says string
	}{
		{"a field the dataset does not publish", definition.Filter{
			Name: "period", Type: definition.Date,
			Bind: map[string]string{"invoices": "created_at"}}, "not a field of it"},

		{"a dataset the report does not read", definition.Filter{
			Name: "period", Type: definition.Date,
			Bind: map[string]string{"shipments": "shipped_at"}}, "does not read"},

		// A control that does nothing, shown to someone who expects it to work.
		{"a filter bound to nothing", definition.Filter{
			Name: "period", Type: definition.Date}, "binds to no dataset"},

		// The field is the one part of a filter predicate that becomes text.
		{"a binding that is not a field name", definition.Filter{
			Name: "period", Type: definition.Date,
			Bind: map[string]string{"invoices": "issued_at; DROP TABLE x"}}, "not a field name"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckFilters([]definition.Filter{c.f}, sets)
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("message %q does not say %q", err, c.says)
			}
		})
	}
}
