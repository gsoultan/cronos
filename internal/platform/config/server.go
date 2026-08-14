package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Server is everything cronosd needs to start.
type Server struct {
	Addr string
	// Definitions is a directory of YAML.
	Definitions string
	// SecretsDir is a directory of files, one per secret, as Docker secrets
	// and Kubernetes projected volumes produce. Empty means the environment
	// alone.
	SecretsDir string
	// Audit is "log" or "off".
	Audit string
	// BehindProxy trusts X-Forwarded-For for rate limiting.
	BehindProxy bool
	// Retention is how long run history is kept. Zero means for ever.
	Retention time.Duration
	// Driver and DSN say where rows live. "sqlite" and a path is the
	// development answer; "postgres" and a URL is the deployed one.
	Driver string
	DSN    string
	// SigningKey signs embed tokens. No default — a default signing key is a
	// shared trust root across every deployment that forgot to set one.
	SigningKey []byte
	// Origins are the host pages allowed to embed. No wildcard; see api.CORS.
	Origins []string
	// AdminKey enables the management API. Absent means the server is
	// read-only, and its endpoints are not mounted at all.
	AdminKey []byte
	// Org and Project are who the admin key acts as. One project, because the
	// file store holds one — see api.AdminKey.
	Org string
	// Projects is `org/project,org/project`. Empty means the single one.
	Projects string
	Project  string
	// StoreDSN puts definitions in a database instead of a directory. Empty
	// keeps the file store, which is one project — see api.AdminKey.
	StoreDSN    string
	StoreDriver string
	// Deliveries is where the file channel writes. Empty disables it.
	Deliveries string
	// Scheduler arms schedules when true. Off by default: two instances both
	// running the same bursts deliver every customer two documents, and
	// deciding which one schedules is a deployment decision rather than a
	// default.
	Scheduler bool
	/*
	   SchedulerTick is how often armed schedules are checked. Zero is a minute.

	   Cron resolves to the minute, so a tick of a minute means a schedule due
	   at 06:00 fires somewhere in the minute after — wherever the process
	   happened to start within it. A deployment that wants statements out at
	   06:00 rather than "some time past six" sets this lower and pays for it in
	   wake-ups, which is the trade to make deliberately rather than to inherit.
	*/
	SchedulerTick time.Duration
	SMTP          SMTP
	// Portal is where the portal is served, for links in email. A separate
	// origin from the API in every real deployment — the portal is a static
	// build behind a CDN — so it cannot be derived from the listen address.
	Portal string
	S3     S3
	// SeedSource names which datasource the seed applies to. Required when
	// several are defined: the seed runs DDL, and picking one would be a guess.
	SeedSource string
	// Seed is a .sql file applied at startup. Development only: it exists so
	// an in-memory database has something in it, and a deployment that points
	// this at a real DSN is running DDL on every restart.
	Seed string
}

// Load reads the environment, filling in the defaults that are safe to have.
func Load() (Server, error) {
	s := Server{
		Addr:        env("CRONOS_ADDR", ":8787"),
		Definitions: env("CRONOS_DEFINITIONS", "examples"),
		Driver:      env("CRONOS_DRIVER", "sqlite"),
		// Shared cache rather than a bare :memory:. Each pooled connection to
		// a plain in-memory SQLite gets its own empty database, so the seed
		// lands on one connection and every concurrent request afterwards
		// finds no tables.
		DSN:        env("CRONOS_DSN", "file:cronos?mode=memory&cache=shared"),
		SigningKey: []byte(os.Getenv("CRONOS_SIGNING_KEY")),
		Seed:       os.Getenv("CRONOS_SEED"),
		SeedSource: os.Getenv("CRONOS_SEED_SOURCE"),
		AdminKey:   []byte(os.Getenv("CRONOS_ADMIN_KEY")),
		// Where ${secret:name} is looked up. Files first when a directory is
		// given, because a mounted file is not visible in /proc to everything
		// running as the same user and does not appear in a crash dump.
		SecretsDir: os.Getenv("CRONOS_SECRETS_DIR"),
		// Where the audit trail goes. "log" by default rather than off: a
		// product whose claim is governed access to somebody else's customers'
		// data, shipping with nothing recorded unless it is switched on, has
		// no answer to the one question an auditor asks. An operator who does
		// not want it says so.
		Audit: env("CRONOS_AUDIT", "log"),
		// Whether X-Forwarded-For may be believed. Only true where something
		// in front sets it, because a caller can send whatever it likes and a
		// rate limit keyed by that is not a limit.
		BehindProxy: os.Getenv("CRONOS_BEHIND_PROXY") == "1",
		// How long run history is kept. Zero is for ever, which is the
		// default: how long a business must be able to show what it sent its
		// customers is a legal question with a different answer in every
		// jurisdiction, and deleting at ninety days by default would be this
		// product answering it on their behalf.
		Retention: duration("CRONOS_HISTORY_RETENTION"),
		Org:       env("CRONOS_ORG", "default"),
		// The projects one process serves, as org/project pairs. Empty is the
		// ordinary deployment: the single project named above, with its
		// definitions where they have always been.
		Projects:      os.Getenv("CRONOS_PROJECTS"),
		Project:       env("CRONOS_PROJECT", "default"),
		StoreDSN:      os.Getenv("CRONOS_STORE_DSN"),
		StoreDriver:   env("CRONOS_STORE_DRIVER", "postgres"),
		Deliveries:    env("CRONOS_DELIVERIES", "deliveries"),
		Scheduler:     os.Getenv("CRONOS_SCHEDULER") == "1",
		SchedulerTick: duration("CRONOS_SCHEDULER_TICK"),
		Portal:        strings.TrimRight(os.Getenv("CRONOS_PORTAL_URL"), "/"),
		SMTP: SMTP{
			Host: os.Getenv("CRONOS_SMTP_HOST"), From: os.Getenv("CRONOS_SMTP_FROM"),
			Username: os.Getenv("CRONOS_SMTP_USER"), Password: os.Getenv("CRONOS_SMTP_PASSWORD"),
		},
		S3: S3{
			Endpoint: os.Getenv("CRONOS_S3_ENDPOINT"), Region: os.Getenv("CRONOS_S3_REGION"),
			AccessKey: os.Getenv("CRONOS_S3_ACCESS_KEY"), SecretKey: os.Getenv("CRONOS_S3_SECRET_KEY"),
		},
	}
	if origins := os.Getenv("CRONOS_ORIGINS"); origins != "" {
		s.Origins = strings.Split(origins, ",")
	}
	if len(s.SigningKey) == 0 {
		// Refused rather than generated. A key generated at startup silently
		// invalidates every token on restart, and one baked in as a default is
		// the same key everyone else's deployment is using.
		return Server{}, fmt.Errorf("config: CRONOS_SIGNING_KEY is required")
	}
	return s, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// SMTP is the mail relay, if one is configured.
//
// A channel with no host is not registered rather than registered and always
// failing. A schedule delivering via a channel nobody configured should say
// "no channel named email" at publish, not at six in the morning.
type SMTP struct {
	Host     string
	From     string
	Username string
	Password string
}

// S3 is object storage, if it is configured.
type S3 struct {
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
}

// Configured reports whether there is enough to register the channel.
func (s SMTP) Configured() bool { return s.Host != "" && s.From != "" }

// Configured reports whether there is enough to register the channel.
func (s S3) Configured() bool { return s.AccessKey != "" && s.SecretKey != "" }

// duration reads a Go duration from the environment, or zero.
//
// A bad value is zero rather than an error, and the difference matters: this
// one governs deletion, and a typo that stopped the server would be safer than
// a typo that deleted more than somebody meant.
func duration(key string) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return 0
	}
	return d
}
