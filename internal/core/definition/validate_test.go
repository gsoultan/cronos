package definition

import (
	"errors"
	"strings"
	"testing"
)

func good() Dataset {
	return Dataset{
		Name:    "invoices",
		Sources: []SourceRef{{Ref: "warehouse"}},
		Query:   "SELECT id, total, currency FROM invoices",
		Params: []Param{
			{Name: "from", Type: Date, Required: true},
			{Name: "status", Type: Enum, Multiple: true, Values: []string{"sent", "paid"}},
		},
		Fields: []Field{
			{Name: "id", Type: "string", Role: Dimension},
			{Name: "currency", Type: "string", Role: Dimension, Hidden: true},
			{Name: "total", Type: "decimal", Role: Measure, Aggregate: "sum", CurrencyField: "currency"},
		},
		RowLevelSecurity: []RowScope{{Predicate: "customer_id = {{ .scope.customer_id }}"}},
	}
}

func TestValidateAccepts(t *testing.T) {
	if err := good().Validate(); err != nil {
		t.Errorf("the reference dataset should validate: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Dataset)
		says string
	}{
		{"a name with spaces", func(d *Dataset) { d.Name = "my invoices" }, "lowercase letters"},
		{"a name that starts with a digit", func(d *Dataset) { d.Name = "2026-invoices" }, "lowercase letters"},
		{"no query", func(d *Dataset) { d.Query = "" }, "no query"},
		{"no source", func(d *Dataset) { d.Sources = nil }, "names no source"},

		{"a param name that is not an identifier", func(d *Dataset) {
			d.Params[0].Name = "from-date"
		}, "underscores"},
		{"the same param twice", func(d *Dataset) {
			d.Params = append(d.Params, Param{Name: "from", Type: String})
		}, "declared twice"},
		{"a type nobody implements", func(d *Dataset) {
			d.Params[0].Type = "datetime"
		}, `unknown type "datetime"`},
		// An enum with no values constrains nothing, which is the reverse of
		// what declaring one was for.
		{"an enum with no values", func(d *Dataset) {
			d.Params[1].Values = nil
		}, "lists no values"},
		{"values on something that is not an enum", func(d *Dataset) {
			d.Params[0].Values = []string{"today"}
		}, "values are not applied"},

		{"no fields", func(d *Dataset) { d.Fields = nil }, "declares no fields"},
		{"a field name that is not an identifier", func(d *Dataset) {
			d.Fields[0].Name = "Total Amount"
		}, "underscores"},
		{"the same field twice", func(d *Dataset) {
			d.Fields = append(d.Fields, Field{Name: "id", Type: "string", Role: Dimension})
		}, "declared twice"},
		{"a role nobody knows", func(d *Dataset) {
			d.Fields[0].Role = "metric"
		}, "want dimension or measure"},
		// Without one, every report invents an aggregate and two reports
		// invent different ones.
		{"a measure with no aggregate", func(d *Dataset) {
			d.Fields[2].Aggregate = ""
		}, "declares no aggregate"},
		{"a currency that is not a field", func(d *Dataset) {
			d.Fields[2].CurrencyField = "ccy"
		}, "which is not a field"},

		// Empty reads as "no restriction" and would silently widen every query.
		{"an empty row scope predicate", func(d *Dataset) {
			d.RowLevelSecurity = []RowScope{{Predicate: ""}}
		}, "empty predicate"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := good()
			c.edit(&d)
			err := d.Validate()
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v, want ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("message %q does not say %q", err, c.says)
			}
		})
	}
}

func TestLookups(t *testing.T) {
	d := good()
	if p, ok := d.Param("status"); !ok || p.Type != Enum {
		t.Errorf("Param(status) = %v, %v", p, ok)
	}
	if _, ok := d.Param("nope"); ok {
		t.Error("Param should report a miss")
	}
	if f, ok := d.Field("total"); !ok || f.Role != Measure {
		t.Errorf("Field(total) = %v, %v", f, ok)
	}
	if _, ok := d.Field("nope"); ok {
		t.Error("Field should report a miss")
	}
}
