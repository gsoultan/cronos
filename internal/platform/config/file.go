package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

/*
Configuration from a file, for a deployment nobody configured by hand.

The environment is still the source of truth and this is the fallback. A
container or a Kubernetes deployment that already sets CRONOS_SIGNING_KEY never
reads this file and behaves exactly as it did before it existed, which is what
makes adding it safe: there is no deployment whose behaviour changes.

What the file is for is the host install where somebody was handed a URL rather
than a shell. cronosd writes it during first-run setup and reads it on every
boot afterwards. It holds the signing key, so it is written 0600 and refused if
it is more readable than that — a secret in a world-readable file is the reason
the systemd unit keeps the environment in a file of its own.
*/

// DefaultPath is where cronosd looks when CRONOS_CONFIG says nothing.
//
// Under the state directory rather than /etc, because the process writes it
// during setup and the systemd unit runs with ProtectSystem=strict — /etc is
// read-only to it, and widening that to let a server rewrite its own
// configuration is a larger permission than this feature is worth.
const DefaultPath = "/var/lib/cronos/config.yaml"

// File is the on-disk shape. A subset of Server: only what somebody would set
// during setup, and nothing derived.
//
// The names are the environment variables without the prefix, lowercased. An
// operator reading this file next to `docs/deploying.md` should not have to
// translate — `signingKey` is CRONOS_SIGNING_KEY and there is nothing else to
// learn.
type File struct {
	Addr        string   `yaml:"addr,omitempty"`
	SigningKey  string   `yaml:"signingKey,omitempty"`
	AdminKey    string   `yaml:"adminKey,omitempty"`
	Org         string   `yaml:"org,omitempty"`
	Project     string   `yaml:"project,omitempty"`
	Definitions string   `yaml:"definitions,omitempty"`
	Origins     []string `yaml:"origins,omitempty"`
	Portal      string   `yaml:"portalUrl,omitempty"`
	BehindProxy bool     `yaml:"behindProxy,omitempty"`
	Audit       string   `yaml:"audit,omitempty"`
	Retention   string   `yaml:"historyRetention,omitempty"`

	// The warehouse a report reads.
	Driver string `yaml:"driver,omitempty"`
	DSN    string `yaml:"dsn,omitempty"`

	// The definition store.
	StoreDriver string `yaml:"storeDriver,omitempty"`
	StoreDSN    string `yaml:"storeDsn,omitempty"`

	Deliveries    string `yaml:"deliveries,omitempty"`
	Scheduler     bool   `yaml:"scheduler,omitempty"`
	SchedulerTick string `yaml:"schedulerTick,omitempty"`
	MetricsAddr   string `yaml:"metricsAddr,omitempty"`

	SMTP struct {
		Host     string `yaml:"host,omitempty"`
		From     string `yaml:"from,omitempty"`
		Username string `yaml:"username,omitempty"`
		Password string `yaml:"password,omitempty"`
	} `yaml:"smtp,omitempty"`

	S3 struct {
		Endpoint  string `yaml:"endpoint,omitempty"`
		Region    string `yaml:"region,omitempty"`
		AccessKey string `yaml:"accessKey,omitempty"`
		SecretKey string `yaml:"secretKey,omitempty"`
	} `yaml:"s3,omitempty"`
}

// Path is where configuration would be read from.
func Path() string {
	if p := os.Getenv("CRONOS_CONFIG"); p != "" {
		return p
	}
	return DefaultPath
}

/*
ReadFile loads the configuration file, or reports that there is none.

A missing file is not an error: most deployments do not have one, and the
caller's next question is "is this a first run", not "why did that fail".
Anything else is an error, including a file that cannot be parsed — a
configuration somebody wrote and cronos silently ignored is worse than one it
refused, because the deployment runs with values nobody chose.
*/
func ReadFile(path string) (File, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, false, nil
	}
	if err != nil {
		return File{}, false, fmt.Errorf("config: reading %s: %w", path, err)
	}

	if err := readableOnlyByOwner(path); err != nil {
		return File{}, false, err
	}

	var f File
	// Unknown fields are refused. Somebody who writes `signingkey` rather than
	// `signingKey` would otherwise get a deployment with no key and an error
	// about a missing one, which names the wrong problem.
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return File{}, false, fmt.Errorf("config: %s is not a cronos configuration: %w", path, err)
	}
	return f, true, nil
}

// readableOnlyByOwner refuses a config file anybody else can read.
//
// It holds the signing key, and a signing key readable by every account on the
// host is one that has effectively been published. Refused rather than
// warned: a warning about a secret is a line in a log nobody reads.
func readableOnlyByOwner(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("config: %s: %w", path, err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		return fmt.Errorf(
			"config: %s is mode %04o and holds the signing key — chmod 600 it", path, mode)
	}
	return nil
}

/*
fill applies the file to a Server, for every setting the environment left empty.

The environment wins, and it wins by having been read first: this only writes
where the value is still zero. That ordering is the whole compatibility story —
a deployment that sets CRONOS_SIGNING_KEY reads no file, and one that sets half
its variables gets the file for the other half rather than a conflict nobody
declared a winner for.
*/
func (f File) fill(s *Server) {
	str := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}
	// The defaulted fields need their default treated as empty, or a file could
	// never set them: Load has already put ":8787" in Addr by the time we get
	// here.
	swap := func(dst *string, v, dflt string) {
		if v != "" && *dst == dflt {
			*dst = v
		}
	}

	if len(s.SigningKey) == 0 && f.SigningKey != "" {
		s.SigningKey = []byte(f.SigningKey)
	}
	if len(s.AdminKey) == 0 && f.AdminKey != "" {
		s.AdminKey = []byte(f.AdminKey)
	}
	if len(s.Origins) == 0 {
		s.Origins = f.Origins
	}

	swap(&s.Addr, f.Addr, ":8787")
	swap(&s.Definitions, f.Definitions, "examples")
	swap(&s.Driver, f.Driver, "sqlite")
	swap(&s.Org, f.Org, "default")
	swap(&s.Project, f.Project, "default")
	swap(&s.StoreDriver, f.StoreDriver, "postgres")
	swap(&s.Deliveries, f.Deliveries, "deliveries")
	swap(&s.Audit, f.Audit, "log")

	str(&s.DSN, f.DSN)
	str(&s.StoreDSN, f.StoreDSN)
	str(&s.Portal, f.Portal)
	str(&s.MetricsAddr, f.MetricsAddr)
	str(&s.SMTP.Host, f.SMTP.Host)
	str(&s.SMTP.From, f.SMTP.From)
	str(&s.SMTP.Username, f.SMTP.Username)
	str(&s.SMTP.Password, f.SMTP.Password)
	str(&s.S3.Endpoint, f.S3.Endpoint)
	str(&s.S3.Region, f.S3.Region)
	str(&s.S3.AccessKey, f.S3.AccessKey)
	str(&s.S3.SecretKey, f.S3.SecretKey)

	// Booleans have no empty, so the file can only turn them on. A deployment
	// that wants one off leaves it out of both places, which is what "off by
	// default" already means.
	if f.BehindProxy {
		s.BehindProxy = true
	}
	if f.Scheduler {
		s.Scheduler = true
	}

	if s.Retention == 0 {
		s.Retention = parseDuration(f.Retention)
	}
	if s.SchedulerTick == 0 {
		s.SchedulerTick = parseDuration(f.SchedulerTick)
	}
}

// parseDuration reads a duration the file supplied, treating anything
// unreadable as unset — the same rule the environment gets, and for the same
// reason: this governs deletion, and a typo that stops the server is safer than
// one that deletes more than somebody meant.
func parseDuration(raw string) time.Duration {
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return 0
	}
	return d
}

/*
Write saves a configuration file, 0600, creating its directory.

Written to a temporary file in the same directory and renamed, so a crash
half-way through leaves the previous configuration rather than a truncated one.
The rename is atomic on the same filesystem, which is why the temporary file is
not in /tmp.
*/
func (f File) Write(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	raw, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	body := append([]byte(header), raw...)

	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("config: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return os.Rename(tmp.Name(), path)
}

const header = `# cronos configuration, written by first-run setup.
#
# Every setting here can also be an environment variable, and the environment
# wins where both are set — see "What has to be set" in docs/deploying.md.
#
# This file holds the signing key. It is mode 0600 and cronos refuses to start
# if that is widened, because a signing key every account on the host can read
# is one that has been published.
`
