/*
Package secret resolves ${secret:name} out of a definition.

A definition is a file somebody commits. A password in one is a password in
their git history for ever, in every fork of it, and in every backup of every
fork — which is why the format has never had a field for one. What it has is a
reference, and this is what turns a reference into the value at the moment a
connection is opened.

The value is never written back. Nothing here stores a resolved DSN, returns
one to the management API, or puts one in a log line: the resolved string
exists between this package and database/sql and nowhere else.
*/
package secret

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// reference matches ${secret:name}, where a name is what an environment
// variable or a file may be called.
//
// Deliberately narrow. A name with a slash in it could name a file outside the
// directory a Files resolver was pointed at, and a name with a space in it is
// a typo somebody should be told about rather than a lookup that fails oddly.
var reference = regexp.MustCompile(`\$\{secret:([A-Za-z0-9_.-]+)\}`)

// Resolver answers with the value behind a name.
type Resolver interface {
	// Secret returns the value, and whether there is one. Absent is not an
	// error here: which of several resolvers holds a name is the caller's
	// question, not the resolver's.
	Secret(name string) (string, bool)
}

/*
Resolve replaces every reference in s.

Every one, or none. A DSN with one of two references resolved is a connection
string that will fail somewhere less obvious than here — half a password, or a
host that is still the literal text `${secret:db_host}` — so a missing name is
an error naming every name that was missing, at startup, rather than a
connection error at six in the morning.
*/
func Resolve(s string, from Resolver) (string, error) {
	if from == nil {
		if missing := Names(s); len(missing) > 0 {
			return "", fmt.Errorf("%w: %s, and no secret source is configured",
				ErrUnresolved, strings.Join(missing, ", "))
		}
		return s, nil
	}

	var missing []string
	out := reference.ReplaceAllStringFunc(s, func(match string) string {
		name := reference.FindStringSubmatch(match)[1]
		value, ok := from.Secret(name)
		if !ok {
			missing = append(missing, name)
			return match
		}
		return value
	})

	if len(missing) > 0 {
		sort.Strings(missing)
		return "", fmt.Errorf("%w: %s", ErrUnresolved, strings.Join(unique(missing), ", "))
	}
	return out, nil
}

// Names lists the references in s, so a caller can say what it will need
// before it needs it.
func Names(s string) []string {
	var out []string
	for _, m := range reference.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return unique(out)
}

func unique(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

/*
Env resolves a name from the environment, as CRONOS_SECRET_<NAME>.

The default, because it is what every container orchestrator already does:
Kubernetes mounts a Secret as environment variables, systemd has
EnvironmentFile, and a developer has an export. Nothing to configure, and no
new place for a credential to live.

The name is upper-cased and dots and dashes become underscores, so
`${secret:warehouse-password}` is CRONOS_SECRET_WAREHOUSE_PASSWORD — an
environment variable somebody can actually set.
*/
type Env struct {
	// Prefix defaults to CRONOS_SECRET_. Configurable so two deployments can
	// share a host without sharing a namespace.
	Prefix string
}

// Secret reads the environment.
func (e Env) Secret(name string) (string, bool) {
	prefix := e.Prefix
	if prefix == "" {
		prefix = "CRONOS_SECRET_"
	}
	return os.LookupEnv(prefix + normalise(name))
}

func normalise(name string) string {
	upper := strings.ToUpper(name)
	return strings.NewReplacer(".", "_", "-", "_").Replace(upper)
}

/*
Files resolves a name from a file in a directory.

What Docker secrets, Kubernetes projected volumes and Vault Agent all produce:
a directory of files, one per secret, each containing the value and nothing
else. Preferred over the environment where it is available, because an
environment variable is visible in /proc to anything running as the same user
and appears in a crash dump.
*/
type Files struct {
	Dir string
}

// Secret reads one file, trimming the trailing newline an editor adds.
func (f Files) Secret(name string) (string, bool) {
	if f.Dir == "" {
		return "", false
	}
	// The name has already been through `reference`, which permits no slashes
	// and no dots-dots — so this cannot leave the directory.
	raw, err := os.ReadFile(f.Dir + "/" + name)
	if err != nil {
		return "", false
	}
	return strings.TrimRight(string(raw), "\r\n"), true
}

// Chain asks each resolver in turn and takes the first answer.
//
// Files before Env in practice, so a deployment can mount the two secrets it
// rotates and leave the rest in the environment.
type Chain []Resolver

// Secret returns the first value any member has.
func (c Chain) Secret(name string) (string, bool) {
	for _, r := range c {
		if r == nil {
			continue
		}
		if v, ok := r.Secret(name); ok {
			return v, true
		}
	}
	return "", false
}

// Map is a resolver over values held in memory. For tests, and for a
// deployment that has exactly one secret and passes it as a flag.
type Map map[string]string

// Secret looks the name up.
func (m Map) Secret(name string) (string, bool) {
	v, ok := m[name]
	return v, ok
}
