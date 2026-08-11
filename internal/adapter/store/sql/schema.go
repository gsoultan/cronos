package sql

import "strings"

// schema creates the tables, if they are not there.
//
// Shipped rather than left to a migration tool, because the first thing anyone
// does with a definition store is store a definition, and a product that
// requires a separate step before that works is a product with a worse first
// ten minutes.
//
// Both tables carry the tenant in their primary key. Two projects may each
// have a `monthly-statement` and they are unrelated — see docs/tenancy.md.
const schema = `
CREATE TABLE IF NOT EXISTS cronos_definitions (
  org        TEXT NOT NULL,
  project    TEXT NOT NULL,
  kind       TEXT NOT NULL,
  name       TEXT NOT NULL,
  version    TEXT NOT NULL,
  body       {{bytes}} NOT NULL,
  updated_at TEXT NOT NULL,
  updated_by TEXT NOT NULL,
  PRIMARY KEY (org, project, kind, name)
);

CREATE TABLE IF NOT EXISTS cronos_definition_versions (
  org        TEXT NOT NULL,
  project    TEXT NOT NULL,
  kind       TEXT NOT NULL,
  name       TEXT NOT NULL,
  version    TEXT NOT NULL,
  body       {{bytes}} NOT NULL,
  created_at TEXT NOT NULL,
  created_by TEXT NOT NULL,
  PRIMARY KEY (org, project, kind, name, version)
);

CREATE TABLE IF NOT EXISTS cronos_runs (
  id             TEXT PRIMARY KEY,
  org            TEXT NOT NULL,
  project        TEXT NOT NULL,
  schedule       TEXT NOT NULL,
  report         TEXT NOT NULL,
  report_version TEXT NOT NULL DEFAULT '',
  output         TEXT NOT NULL DEFAULT '',
  period_start   TEXT NOT NULL DEFAULT '',
  period_end     TEXT NOT NULL DEFAULT '',
  triggered_by   TEXT NOT NULL DEFAULT '',
  started_at     TEXT NOT NULL,
  finished_at    TEXT,
  recipients     INTEGER NOT NULL DEFAULT 0,
  delivered      INTEGER NOT NULL DEFAULT 0,
  status         TEXT NOT NULL,
  error          TEXT NOT NULL DEFAULT ''
);

-- Listing a project's runs newest-first is the only query anyone makes of
-- this table by hand, and it is the one a support conversation starts with.
CREATE INDEX IF NOT EXISTS cronos_runs_by_project
  ON cronos_runs (org, project, started_at DESC);

CREATE TABLE IF NOT EXISTS cronos_deliveries (
  run_id      TEXT NOT NULL,
  recipient   TEXT NOT NULL,
  channel     TEXT NOT NULL,
  destination TEXT NOT NULL DEFAULT '',
  filename    TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL,
  attempts    INTEGER NOT NULL DEFAULT 0,
  bytes       INTEGER NOT NULL DEFAULT 0,
  error       TEXT NOT NULL DEFAULT '',
  at          TEXT NOT NULL,
  PRIMARY KEY (run_id, recipient, channel)
);

-- "What did this customer receive?" is the question the table exists for, and
-- without this it is a scan of every delivery ever made.
CREATE INDEX IF NOT EXISTS cronos_deliveries_by_recipient
  ON cronos_deliveries (recipient, at DESC);

CREATE TABLE IF NOT EXISTS cronos_users (
  id         TEXT PRIMARY KEY,
  -- Lowercased on the way in. Somebody typing Dewi@Acme.example at six in the
  -- morning is the same person as dewi@acme.example, and a unique index that
  -- disagrees lets them create a second account instead of failing to log in.
  email      TEXT NOT NULL UNIQUE,
  name       TEXT NOT NULL DEFAULT '',
  password   TEXT NOT NULL,
  org        TEXT NOT NULL,
  project    TEXT NOT NULL,
  role       TEXT NOT NULL,
  created_at TEXT NOT NULL,
  last_seen  TEXT,
  disabled   BOOLEAN NOT NULL DEFAULT FALSE
);

-- A share is a token somebody handed out, and the record that lets them take
-- it back. The token itself is not here: it is signed, so it needs no storage
-- to be valid, and keeping a copy would put a live credential in a table that
-- is backed up, replicated and read by whoever can read the others.
CREATE TABLE IF NOT EXISTS cronos_shares (
  id         TEXT PRIMARY KEY,
  org        TEXT NOT NULL,
  project    TEXT NOT NULL,
  report     TEXT NOT NULL,
  -- The row constraint the recipient sees through, as JSON. Copied from the
  -- sharer at the moment of sharing: a link that widened when its author's
  -- own access widened would be a grant nobody made.
  scope      TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL,
  created_at TEXT NOT NULL,
  -- Null means it does not expire, which is a choice somebody made rather
  -- than a default: EXPIRIES offers Never and hiding it would not stop it.
  expires_at TEXT,
  revoked_at TEXT
);

-- Listing a project's shares is the whole read pattern; by id is the primary
-- key already.
CREATE INDEX IF NOT EXISTS cronos_shares_by_project
  ON cronos_shares (org, project, created_at DESC);
`

// Schema is the DDL for a driver.
//
// Almost identical, and the almost is the point. SQLite has BLOB and Postgres
// has BYTEA, and neither knows the other's name — a schema written for one
// does not create tables on the other, it fails on the first statement. That
// is the class of thing testing against a single database cannot show, and it
// was true here until a Postgres ran the same code.
func Schema(driver string) string {
	bytes := "BLOB"
	if driver == "postgres" || driver == "pgx" {
		bytes = "BYTEA"
	}
	return strings.ReplaceAll(schema, "{{bytes}}", bytes)
}
