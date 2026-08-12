package definition

import (
	"errors"
	"strings"
	"testing"
	"time"
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

// The pool's own comment said it was bounded because somebody else operates
// that database. It was not: every setting was applied only when a definition
// named it, so a source that said nothing got database/sql's defaults — and
// the first of those is unlimited connections.
func TestAPoolThatSaysNothingIsStillBounded(t *testing.T) {
	var unset Pool

	if unset.Open() != DefaultMaxOpen {
		t.Errorf("MaxOpen is %d, want %d", unset.Open(), DefaultMaxOpen)
	}
	// Matched to Open rather than to database/sql's two: keeping two under
	// load means a connection is opened and closed for almost every query — a
	// handshake, a TLS negotiation and an authentication round trip, per block.
	if unset.Idle() != unset.Open() {
		t.Errorf("MaxIdle is %d, want %d", unset.Idle(), unset.Open())
	}
	// The one nobody thinks of until a failover hands out a dead connection.
	if unset.LifetimeOf() != DefaultMaxLifetime {
		t.Errorf("MaxLifetime is %s", unset.LifetimeOf())
	}
	if unset.IdleFor() != DefaultMaxIdleTime {
		t.Errorf("MaxIdleTime is %s", unset.IdleFor())
	}
}

// And a deployment that owns the database it reads can say so.
func TestAPoolThatSaysSomethingIsHonoured(t *testing.T) {
	set := Pool{
		MaxOpen:     64,
		MaxIdle:     8,
		MaxIdleTime: Duration(90 * time.Second),
		MaxLifetime: Duration(2 * time.Hour),
	}

	if set.Open() != 64 || set.Idle() != 8 {
		t.Errorf("open %d idle %d", set.Open(), set.Idle())
	}
	if set.IdleFor() != 90*time.Second || set.LifetimeOf() != 2*time.Hour {
		t.Errorf("idle for %s, lifetime %s", set.IdleFor(), set.LifetimeOf())
	}
}

// Eight on every machine was two things at once: a fifth of the throughput
// unused on fifteen cores, and four typesetters per core on a container with
// two. The bounds are what make one number safe on both.
func TestBurstConcurrencyFollowsTheMachineWithinBounds(t *testing.T) {
	got := DefaultConcurrency()

	if got < minConcurrency {
		t.Errorf("%d is below the floor of %d — a worker spends most of its life "+
			"waiting, so even one core wants several in flight", got, minConcurrency)
	}
	if got > maxConcurrency {
		t.Errorf("%d is above the ceiling of %d — a burst is not supposed to be the "+
			"only thing this process can do", got, maxConcurrency)
	}
}

// A schedule that knows its own database says so, and is not second-guessed.
func TestAStatedConcurrencyWins(t *testing.T) {
	stated := BurstSpec{Concurrency: 3}
	if stated.Workers() != 3 {
		t.Fatalf("workers is %d", stated.Workers())
	}
}
