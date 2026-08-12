package secret_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/platform/secret"
)

// A definition is a file somebody commits. These are the rules that keep a
// password out of it, and out of everything downstream of it.

func TestAReferenceBecomesItsValue(t *testing.T) {
	got, err := secret.Resolve(
		"postgres://cronos:${secret:warehouse_password}@db.internal:5432/analytics",
		secret.Map{"warehouse_password": "hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://cronos:hunter2@db.internal:5432/analytics"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

// Every one or none. A DSN with one of two resolved fails somewhere less
// obvious than here — half a password, or a host still spelled ${secret:…}.
func TestOneMissingNameRefusesTheWholeString(t *testing.T) {
	_, err := secret.Resolve(
		"postgres://${secret:user}:${secret:pass}@h/db",
		secret.Map{"user": "cronos"})

	if !errors.Is(err, secret.ErrUnresolved) {
		t.Fatalf("want unresolved, got %v", err)
	}
	// Named, so somebody can go and set it rather than guess which.
	if !strings.Contains(err.Error(), "pass") {
		t.Fatalf("the error does not name it: %v", err)
	}
	if strings.Contains(err.Error(), "cronos") {
		t.Fatalf("the error leaked a value it did resolve: %v", err)
	}
}

// Every missing name at once. Fixing one and being told about the next is a
// deploy each time.
func TestEveryMissingNameIsNamedAtOnce(t *testing.T) {
	_, err := secret.Resolve("${secret:a}${secret:b}${secret:c}", secret.Map{"b": "x"})
	for _, name := range []string{"a", "c"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("%q is missing from %v", name, err)
		}
	}
}

// A string with no references is not a string that needs a resolver.
func TestAStringWithoutReferencesPassesThrough(t *testing.T) {
	const dsn = "file:cronos-demo?mode=memory&cache=shared"
	got, err := secret.Resolve(dsn, nil)
	if err != nil || got != dsn {
		t.Fatalf("got %q, %v", got, err)
	}
}

// But one with references and nowhere to look them up is a deployment that
// should not start.
func TestNoResolverAndAReferenceIsAnError(t *testing.T) {
	_, err := secret.Resolve("${secret:pass}", nil)
	if !errors.Is(err, secret.ErrUnresolved) {
		t.Fatalf("want unresolved, got %v", err)
	}
	if !strings.Contains(err.Error(), "no secret source is configured") {
		t.Fatalf("the error does not say why: %v", err)
	}
}

func TestTheEnvironmentIsSpelledTheWayAnOperatorWouldSetIt(t *testing.T) {
	t.Setenv("CRONOS_SECRET_WAREHOUSE_PASSWORD", "hunter2")

	// Dots and dashes become underscores, because an environment variable
	// cannot have them.
	for _, name := range []string{"warehouse_password", "warehouse-password", "WAREHOUSE.PASSWORD"} {
		got, ok := secret.Env{}.Secret(name)
		if !ok || got != "hunter2" {
			t.Errorf("%q resolved to %q, %v", name, got, ok)
		}
	}
}

func TestAFileIsReadWithoutTheNewlineAnEditorAdds(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "warehouse_password"), []byte("hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, ok := secret.Files{Dir: dir}.Secret("warehouse_password")
	if !ok || got != "hunter2" {
		t.Fatalf("got %q, %v", got, ok)
	}
}

// Files first, then the environment: a deployment mounts the two it rotates
// and leaves the rest where the orchestrator already put them.
func TestTheChainTakesTheFirstAnswer(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shared"), []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CRONOS_SECRET_SHARED", "from-env")
	t.Setenv("CRONOS_SECRET_ONLY_ENV", "from-env")

	chain := secret.Chain{secret.Files{Dir: dir}, secret.Env{}}
	if v, _ := chain.Secret("shared"); v != "from-file" {
		t.Fatalf("shared resolved to %q", v)
	}
	if v, _ := chain.Secret("only_env"); v != "from-env" {
		t.Fatalf("only_env resolved to %q", v)
	}
}

// A name that could name a file outside the directory is not a name.
func TestAReferenceCannotEscapeTheSecretsDirectory(t *testing.T) {
	for _, hostile := range []string{
		"${secret:../../etc/passwd}", "${secret:/etc/passwd}", "${secret:a/b}",
	} {
		if names := secret.Names(hostile); len(names) != 0 {
			t.Errorf("%q parsed as a reference to %v", hostile, names)
		}
	}
}

// What a caller needs before it needs it, so a deployment can be checked
// rather than attempted.
func TestNamesListsWhatAStringWillNeed(t *testing.T) {
	got := secret.Names("${secret:b}//${secret:a}:${secret:b}")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
}
