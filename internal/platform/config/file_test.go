package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gsoultan/cronos/internal/platform/config"
)

/*
Configuration from a file, and the environment still winning.

Two properties carry this feature. The environment wins, which is what makes it
safe to add — a container that sets CRONOS_SIGNING_KEY reads no file and behaves
exactly as it did before, so there is no deployment whose behaviour changes. And
the file holds the signing key, so a file anybody else can read is refused
rather than warned about: a warning about a secret is a line in a log nobody
reads.
*/

// wrote puts a config file somewhere the test owns and points cronos at it.
func wrote(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CRONOS_CONFIG", path)
	return path
}

func TestAFileSuppliesWhatTheEnvironmentDidNot(t *testing.T) {
	t.Setenv("CRONOS_SIGNING_KEY", "")
	wrote(t, `
signingKey: from-the-file-at-least-32-bytes-xx
org: acme
project: finance
storeDsn: postgres://localhost/cronos
behindProxy: true
historyRetention: 2160h
`)

	s, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Unconfigured {
		t.Error("a deployment configured by file reports itself unconfigured")
	}
	if string(s.SigningKey) != "from-the-file-at-least-32-bytes-xx" {
		t.Errorf("signing key is %q", s.SigningKey)
	}
	if s.Org != "acme" || s.Project != "finance" {
		t.Errorf("tenancy is %s/%s, want acme/finance", s.Org, s.Project)
	}
	if s.StoreDSN != "postgres://localhost/cronos" {
		t.Errorf("store dsn is %q", s.StoreDSN)
	}
	if !s.BehindProxy {
		t.Error("behindProxy from the file was not applied")
	}
	if s.Retention == 0 {
		t.Error("historyRetention from the file was not applied")
	}
	if !s.FromFile {
		t.Error("the server does not record that it read a file")
	}
}

/*
The environment wins, every time.

This is the compatibility property, and it is worth asserting field by field
rather than once: a fill() that gets one of these backwards is a deployment
reading a value nobody set, and the symptom is somebody's staging signing key
in production.
*/
func TestTheEnvironmentBeatsTheFile(t *testing.T) {
	t.Setenv("CRONOS_SIGNING_KEY", "from-the-environment-32-bytes-xxxx")
	t.Setenv("CRONOS_ORG", "env-org")
	t.Setenv("CRONOS_ADDR", ":9999")
	t.Setenv("CRONOS_STORE_DSN", "postgres://env/cronos")
	wrote(t, `
signingKey: from-the-file-at-least-32-bytes-xx
org: file-org
addr: ":1111"
storeDsn: postgres://file/cronos
`)

	s, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, got, want string }{
		{"signing key", string(s.SigningKey), "from-the-environment-32-bytes-xxxx"},
		{"org", s.Org, "env-org"},
		{"addr", s.Addr, ":9999"},
		{"store dsn", s.StoreDSN, "postgres://env/cronos"},
	} {
		if c.got != c.want {
			t.Errorf("%s is %q, want the environment's %q", c.name, c.got, c.want)
		}
	}
}

// And a file can still set what the environment left alone, in the same load.
// Half from each is the ordinary case for somebody who set a couple of
// variables in a unit and let setup write the rest.
func TestTheTwoSourcesMix(t *testing.T) {
	t.Setenv("CRONOS_SIGNING_KEY", "from-the-environment-32-bytes-xxxx")
	t.Setenv("CRONOS_ORG", "")
	wrote(t, "org: file-org\nproject: file-project\n")

	s, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if string(s.SigningKey) != "from-the-environment-32-bytes-xxxx" {
		t.Errorf("signing key is %q", s.SigningKey)
	}
	if s.Org != "file-org" || s.Project != "file-project" {
		t.Errorf("tenancy is %s/%s, want the file's", s.Org, s.Project)
	}
}

/*
A config file anybody can read is refused.

It holds the signing key. A key every account on the host can read is one that
has effectively been published, and starting anyway would mean the deployment
is compromised and running.
*/
func TestAWorldReadableConfigIsRefused(t *testing.T) {
	t.Setenv("CRONOS_SIGNING_KEY", "")
	path := wrote(t, "org: acme\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load()
	if err == nil {
		t.Fatal("a world-readable configuration was accepted")
	}
	// Named, because the fix is one command and an operator should not have to
	// work out which.
	if !strings.Contains(err.Error(), "600") {
		t.Errorf("the error does not say what to do: %v", err)
	}
}

// A missing file is a first run, not a failure. Most deployments have no file
// and the caller's next question is "is this a first run", not "why did that
// fail".
func TestAMissingFileIsNotAnError(t *testing.T) {
	t.Setenv("CRONOS_SIGNING_KEY", "")
	t.Setenv("CRONOS_CONFIG", filepath.Join(t.TempDir(), "nothing-here.yaml"))

	s, err := config.Load()
	if err != nil {
		t.Fatalf("a missing configuration file was an error: %v", err)
	}
	if !s.Unconfigured || s.FromFile {
		t.Errorf("unconfigured=%v fromFile=%v, want true and false", s.Unconfigured, s.FromFile)
	}
}

/*
A file that cannot be parsed is refused rather than ignored.

A configuration somebody wrote and cronos silently skipped is worse than one it
refused: the deployment runs, with values nobody chose, and the file on disk
says otherwise.
*/
func TestAnUnreadableFileIsRefused(t *testing.T) {
	t.Setenv("CRONOS_SIGNING_KEY", "")

	for _, c := range []struct{ name, body string }{
		{"not yaml", "{{{ this is not yaml"},
		// A key somebody typed slightly wrong would otherwise leave the
		// deployment with no signing key and an error naming the wrong problem.
		{"a misspelled field", "signingkey: lowercase-k-is-not-the-field\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			wrote(t, c.body)
			if _, err := config.Load(); err == nil {
				t.Error("accepted a configuration file it could not read")
			}
		})
	}
}

/* -- writing --------------------------------------------------------------- */

func TestAWrittenFileIsPrivateAndReadsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")

	var f config.File
	f.SigningKey = "written-by-setup-at-least-32-byte"
	f.Org = "acme"
	f.Project = "finance"
	if err := f.Write(path); err != nil {
		t.Fatal(err)
	}

	// 0600, because it holds the key. Written into a directory that did not
	// exist, because setup runs before anybody has made one.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("written mode is %04o, want 0600", mode)
	}

	back, found, err := config.ReadFile(path)
	if err != nil || !found {
		t.Fatalf("reading back: found=%v err=%v", found, err)
	}
	if back.SigningKey != f.SigningKey || back.Org != "acme" {
		t.Errorf("read back %+v", back)
	}

	// And it says what it is, for whoever opens it next: where it came from,
	// that the environment overrides it, and why it is 0600.
	raw, _ := os.ReadFile(path)
	for _, want := range []string{"first-run setup", "environment", "signing key"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the written file does not mention %q:\n%s", want, raw)
		}
	}
}

// Overwriting keeps the previous file until the new one is complete, so a
// crash part-way through leaves a configuration rather than a truncated one.
func TestWritingOverAnExistingFileIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	first := config.File{Org: "first"}
	if err := first.Write(path); err != nil {
		t.Fatal(err)
	}
	second := config.File{Org: "second"}
	if err := second.Write(path); err != nil {
		t.Fatal(err)
	}

	back, _, err := config.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Org != "second" {
		t.Errorf("org is %q, want second", back.Org)
	}
	// No temporary file left behind to be mistaken for a configuration.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only config.yaml", names)
	}
}
