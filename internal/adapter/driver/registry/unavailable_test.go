package registry_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/adapter/driver/registry"
	"github.com/gsoultan/cronos/internal/core/definition"
	"github.com/gsoultan/cronos/internal/platform/secret"

	_ "modernc.org/sqlite"
)

/*
One source this build cannot open, and the others that it can.

Opening used to be fatal, on the reasoning that a server starting with three of
its four warehouses unreachable serves three-quarters of its reports and fails
the rest at six in the morning. The reasoning described a failure that cannot
happen here — sql.Open does not connect, so an unreachable warehouse opens
perfectly well and fails at query time. What actually reaches it is a driver
this build has no import for, and a ${secret:…} the deployment has not set.

Both are publishable. `driver: mysql` was accepted by definition.Validate long
before any MySQL driver was registered, so an editor could publish one, get a
200, and take the deployment down at its next start — with the API down, so the
only way to remove it was a prompt on the database.
*/
func TestASourceThatWillNotOpenDoesNotTakeTheOthersWithIt(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{
		{Name: "warehouse", Driver: "sqlite", DSN: "file:open-test?mode=memory&cache=shared"},
		{Name: "lake", Driver: "duckdb", DSN: "file:/nowhere.duckdb"},
	}, secret.Chain{}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("one unopenable source failed the whole registry: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() })

	if _, ok := reg.DB("warehouse"); !ok {
		t.Fatal("the source that opens is not registered")
	}
	if _, ok := reg.DB("lake"); ok {
		t.Fatal("a source that could not be opened is registered, so a query would find a nil pool")
	}

	why := reg.Unavailable()
	if len(why) != 1 {
		t.Fatalf("reported %d reasons, and one source could not be opened", len(why))
	}
	// Named, because an operator reading the log has to know which definition
	// to open — and told the driver, because that is what is actually wrong.
	for _, want := range []string{"lake", "duckdb"} {
		if !strings.Contains(why[0].Error(), want) {
			t.Fatalf("the reason does not mention %q: %v", want, why[0])
		}
	}
}

/*
And the DSN is not in it.

This printed the definition's own DSN text, on the reasoning that the
unresolved version says ${secret:…} where the password goes. True when a
definition uses a secret reference, false when somebody wrote the password
inline — which is allowed, and is what a first deployment does. The error goes
to the startup log, so that is a credential in the log of every instance that
failed to start.
*/
func TestTheReasonDoesNotCarryTheDSN(t *testing.T) {
	const password = "Xq7-warehouse-secret-Zt2"

	reg, err := registry.New([]definition.DataSource{
		{Name: "lake", Driver: "duckdb", DSN: "duckdb://user:" + password + "@host/db"},
	}, secret.Chain{}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reg.Close() })

	why := reg.Unavailable()
	if len(why) != 1 {
		t.Fatalf("reported %d reasons", len(why))
	}
	if strings.Contains(why[0].Error(), password) {
		t.Fatalf("the reason carries the password, and it is written to the startup log:\n  %v", why[0])
	}
}

// A secret the deployment has not set is the other way this happens, and it is
// the more likely one: the same definition, promoted to an environment where
// that name is not filled in.
func TestAMissingSecretSkipsTheSourceRatherThanTheProcess(t *testing.T) {
	reg, err := registry.New([]definition.DataSource{
		{Name: "warehouse", Driver: "sqlite", DSN: "file:secret-test?mode=memory&cache=shared"},
		{Name: "crm", Driver: "postgres", DSN: "postgres://u:${secret:nobody-set-this}@h/db"},
	}, secret.Chain{}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("a missing secret stopped the whole registry: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() })

	if _, ok := reg.DB("warehouse"); !ok {
		t.Fatal("the source that needs no secret is not registered")
	}
	if len(reg.Unavailable()) != 1 {
		t.Fatalf("reported %d reasons, and one secret was missing", len(reg.Unavailable()))
	}
}
