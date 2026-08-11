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
`
