//go:build duckdb

package duckdb

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gsoultan/cronos/internal/core/definition"
)

// alias is the shape a mount name may take.
//
// The alias becomes a SQL identifier in an ATTACH, which is the one place a
// definition's text reaches a statement here. definition.Validate enforces the
// same shape; this is the check that makes it structural rather than trusted.
var alias = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// mount is the SQL that makes one source readable under its alias.
func mount(name string, src definition.DataSource) (string, error) {
	if !alias.MatchString(name) {
		return "", fmt.Errorf("duckdb: %q is not a mount name", name)
	}
	switch src.Driver {
	case "postgres":
		return attach("postgres", name, src.DSN), nil
	case "mysql":
		return attach("mysql", name, src.DSN), nil
	case "sqlite":
		return attach("sqlite", name, src.DSN), nil
	case "duckdb":
		// Already this engine. Attaching a DuckDB file needs no extension and
		// no type.
		return fmt.Sprintf("ATTACH %s AS %s (READ_ONLY);", quote(src.DSN), name), nil
	case "object-store":
		return view(name, src)
	}
	return "", fmt.Errorf("duckdb: cannot mount a %s source", src.Driver)
}

// attach loads the extension and mounts the database, read-only.
//
// INSTALL then LOAD every time: both are idempotent, and the alternative is
// tracking which extensions a connection has seen — state that is wrong the
// first time a pooled connection is replaced.
func attach(extension, name, dsn string) string {
	return fmt.Sprintf(
		"INSTALL %s; LOAD %s; ATTACH %s AS %s (TYPE %s, READ_ONLY);",
		extension, extension, quote(dsn), name, strings.ToUpper(extension))
}

// view exposes an object store as a table.
//
// A view rather than an attachment, because a bucket of Parquet is not a
// database: there is no catalogue to mount, only files to read. The glob is
// recursive so a lake partitioned by date does not need one view per day.
func view(name string, src definition.DataSource) (string, error) {
	reader, ok := readers[src.Format]
	if !ok {
		return "", fmt.Errorf("duckdb: cannot read %q from an object store", src.Format)
	}
	uri := strings.TrimSuffix(src.URI, "/") + "/**/*." + src.Format
	return fmt.Sprintf("CREATE OR REPLACE VIEW %s AS SELECT * FROM %s(%s);",
		name, reader, quote(uri)), nil
}

var readers = map[string]string{
	"parquet": "read_parquet",
	"csv":     "read_csv_auto",
	"json":    "read_json_auto",
}

// quote makes a string literal.
//
// DSNs and URIs come from a datasource definition, which an operator wrote —
// but an operator is not a reason to concatenate, and a password containing an
// apostrophe would otherwise end the literal and the statement with it.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
