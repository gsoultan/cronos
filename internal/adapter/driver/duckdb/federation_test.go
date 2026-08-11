//go:build duckdb

package duckdb

import (
	"context"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/core/definition"
)

func TestMountSQL(t *testing.T) {
	cases := []struct {
		name   string
		src    definition.DataSource
		mount  string
		expect []string
	}{
		{"postgres", definition.DataSource{Driver: "postgres", DSN: "postgres://u:p@h/db"}, "warehouse",
			[]string{"INSTALL postgres", "LOAD postgres",
				"ATTACH 'postgres://u:p@h/db' AS warehouse (TYPE POSTGRES, READ_ONLY)"}},

		{"a parquet lake", definition.DataSource{
			Driver: "object-store", URI: "s3://acme-lake/events/", Format: "parquet"}, "events",
			[]string{"CREATE OR REPLACE VIEW events",
				"read_parquet('s3://acme-lake/events/**/*.parquet')"}},

		{"a csv bucket", definition.DataSource{
			Driver: "object-store", URI: "s3://b/x", Format: "csv"}, "sheets",
			[]string{"read_csv_auto('s3://b/x/**/*.csv')"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := mount(c.mount, c.src)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range c.expect {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

// A reporting tool is handed credentials to somebody's production warehouse.
// The difference between one that cannot write and one that merely does not is
// the difference between a reviewable risk and an unreviewable one.
func TestEveryAttachmentIsReadOnly(t *testing.T) {
	for _, driver := range []string{"postgres", "mysql", "sqlite", "duckdb"} {
		got, err := mount("src", definition.DataSource{Driver: driver, DSN: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "READ_ONLY") {
			t.Errorf("%s mounts writable:\n%s", driver, got)
		}
	}
}

// A password with an apostrophe would otherwise end the literal, and the
// statement with it.
func TestDSNsAreQuotedNotConcatenated(t *testing.T) {
	got, err := mount("w", definition.DataSource{
		Driver: "postgres", DSN: "postgres://u:pa'ss@h/db"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "'postgres://u:pa''ss@h/db'") {
		t.Errorf("the apostrophe was not doubled:\n%s", got)
	}
}

// The alias becomes a SQL identifier, which is the one place a definition's
// text reaches a statement here.
func TestAMountNameMustBeAnIdentifier(t *testing.T) {
	for _, bad := range []string{"warehouse; DROP TABLE x", "1warehouse", "ware-house", ""} {
		if _, err := mount(bad, definition.DataSource{Driver: "postgres", DSN: "x"}); err == nil {
			t.Errorf("accepted %q as a mount name", bad)
		}
	}
}

func TestAFederationExecutes(t *testing.T) {
	f, err := Open(context.Background(), nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// A query DuckDB answers on its own, which proves the driver is wired
	// without needing an extension download or a network.
	var n int
	if err := f.db.QueryRow("SELECT count(*) FROM range(10)").Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 10 {
		t.Errorf("got %d, want 10", n)
	}
}

func TestMountingAnUnknownDriverIsAnError(t *testing.T) {
	if _, err := mount("x", definition.DataSource{Driver: "oracle", DSN: "x"}); err == nil {
		t.Error("a driver nobody implemented was mounted")
	}
}
