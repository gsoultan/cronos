package boot

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

/*
A driver's own error sentence, and the password it was handed.

POST /v1/datasources/{name}/test returns the driver's message to the browser
verbatim, deliberately: only the driver knows whether this was a wrong
password, a closed port or a database that is not there, and "could not
connect" is the same three words for all three. That is the right call and it
rests on something nobody here controls — whether the driver quotes the DSN it
was given back at you.

Today all three redact or omit it: pgx prints `postgres://user:xxxxxx@host`,
go-mssqldb names the host and not the string, sqlite has nothing to say. None
of that is promised by any of them. A dependency bump that starts echoing the
connection string turns an endpoint an editor can call into a way to read the
warehouse password out of a project they can already edit — and every existing
test would still pass, because the endpoint's behaviour would not have changed
at all.

The parse failures are the interesting half. A driver that cannot make sense
of a string is the one most likely to show you the string.
*/
func TestNoDriverPutsThePasswordInItsError(t *testing.T) {
	t.Parallel()

	// Distinctive, so a substring match cannot be a coincidence, and shaped
	// like something somebody would actually set.
	const pw = "Xq7-warehouse-secret-Zt2"

	for _, c := range []struct {
		what   string
		driver string
		dsn    string
	}{
		// Refused: the connection is attempted and fails.
		{"postgres, refused", "pgx",
			"postgres://cronos:" + pw + "@127.0.0.1:1/cronos?sslmode=disable"},
		{"sqlserver, refused", "sqlserver",
			"sqlserver://cronos:" + pw + "@127.0.0.1:1?database=cronos"},

		// Unparseable: the driver never gets as far as a socket, and has the
		// whole string in hand at the moment it gives up.
		{"postgres, bad port", "pgx",
			"postgres://cronos:" + pw + "@127.0.0.1:notaport/cronos"},
		{"postgres, bad option", "pgx",
			"postgres://cronos:" + pw + "@127.0.0.1:5432/c?sslmode=nonsense"},
		{"sqlserver, bad port", "sqlserver",
			"sqlserver://cronos:" + pw + "@127.0.0.1:notaport?database=c"},
		{"sqlserver, bad option", "sqlserver",
			"sqlserver://cronos:" + pw + "@127.0.0.1:1?dial+timeout=nope"},

		// sqlite takes its settings in the string too.
		{"sqlite, unopenable", "sqlite",
			"file:/no-such-directory-here/db?_pragma=busy_timeout(" + pw + ")"},
	} {
		t.Run(c.what, func(t *testing.T) {
			t.Parallel()

			// Open and Ping both, because which one refuses varies by driver:
			// pgx defers parsing to the first connection, go-mssqldb does it
			// in Open. Whichever speaks, it is the sentence the browser gets.
			db, err := sql.Open(c.driver, c.dsn)
			if err != nil {
				assertQuiet(t, "Open", err, pw)
				return
			}
			defer func() { _ = db.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err = db.PingContext(ctx)
			if err == nil {
				t.Fatalf("%s connected — this DSN was meant to fail", c.what)
			}
			assertQuiet(t, "Ping", err, pw)
		})
	}
}

func assertQuiet(t *testing.T, from string, err error, pw string) {
	t.Helper()
	if strings.Contains(err.Error(), pw) {
		t.Fatalf("%s returned the password in its error, and /v1/datasources/{name}/test "+
			"sends that to a browser:\n  %s", from, err)
	}
}
