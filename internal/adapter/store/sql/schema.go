package sql

// Schema creates the tables, if they are not there.
//
// Shipped rather than left to a migration tool, because the first thing anyone
// does with a definition store is store a definition, and a product that
// requires a separate step before that works is a product with a worse first
// ten minutes.
//
// Both tables carry the tenant in their primary key. Two projects may each
// have a `monthly-statement` and they are unrelated — see docs/tenancy.md.
const Schema = `
CREATE TABLE IF NOT EXISTS cronos_definitions (
  org        TEXT NOT NULL,
  project    TEXT NOT NULL,
  kind       TEXT NOT NULL,
  name       TEXT NOT NULL,
  version    TEXT NOT NULL,
  body       BLOB NOT NULL,
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
  body       BLOB NOT NULL,
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
  disabled   INTEGER NOT NULL DEFAULT 0
);
`
