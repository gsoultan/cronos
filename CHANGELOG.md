# Changelog

What changed between builds, for somebody deciding whether to upgrade.

Every entry is written for the person running cronos rather than the person who
changed it: what behaviour is different, what a deployment has to do about it,
and what breaks if they do nothing. A line nobody can act on does not belong
here — the git history is where the reasoning lives.

`cronosd -version` prints the build. It is a tag on a release and the commit
otherwise, with `+dirty` where the tree had uncommitted changes. The container
answers the same, provided the image was built with `--build-arg
CRONOS_VERSION=…`; see "The image" in [docs/deploying.md](docs/deploying.md).

Versions are `MAJOR.MINOR.PATCH`. Before 1.0 the minor is the roadmap release in
[docs/product.md](docs/product.md), and a minor may change behaviour that a
deployment depends on — each one says so under **Upgrading** below.

---

## v0.5.0 — 2026-08-17

The first tagged release. Everything through the roadmap's v0.5 is here: the
engine, paginated documents, the scheduler and bursting, the embed surface and
its tokens, and the authoring portal. Most of v1.0 is here too — leader
election, SSO, audit, and the deployment documentation — with one exception,
below.

**The engine.** YAML definitions with bound parameters and structural row-level
security, over Postgres, MySQL, SQL Server, SQLite and object stores. (MySQL was
accepted by every layer and unreachable until this release, for want of one
blank import.) Federation
across engines behind `-tags duckdb`: one query joining a customer list in one
database to invoices in another, every source attached read-only. Results are
bounded everywhere and streamed almost everywhere — a render never holds more
than a page, whether the table has fifty rows or two million.

**Documents.** Paginated PDF through Typst, with grouping, page breaks,
subtotals and running headers. Spreadsheet and CSV output. A hundred thousand
rows is the export ceiling and costs about 32MB while the workbook is written.

**Delivery.** A cron scheduler with per-project isolation and leader election,
so every replica can be armed and exactly one fires. Bursting renders one
document per recipient — 560–650 PDFs per second on the machine in
[docs/deploying.md](docs/deploying.md). Email, Telegram and object-store
channels, retries per recipient, alerts to a person when a run fails, and run
history that says what was delivered to whom. A burst cut in half resumes
without sending anybody two.

**Embedding.** Audience-scoped tokens, a 3.0 KB embed component, and tenancy
that holds: an end customer's token reaches its own report and nothing else, row
scope cannot be widened from the request body, and one organisation cannot see
another's anything.

**Authoring.** A portal with a report builder, dataset browser, live preview and
schedule editor. Installable, offline-capable shell, virtualised tables, and row
work on a web worker.

**Identity.** Passwords, invitations by email, password reset, two-factor
authentication with recovery codes, a project security policy that can require
it of everyone, and OIDC single sign-on with single log-out in `ee/`. Ending
every session an account holds takes effect within five seconds rather than at
token expiry.

**Operations.** Liveness and readiness that ask real questions, Prometheus
metrics including scheduler health and datasource pool utilisation, an audit of
who read what and what was refused, retention for run history and deliveries,
a container image, and a restore drill that CI runs on every push.

### Not in this release

- **No `.jrxml` importer.** Migrating an existing JasperReports estate is v1.0's
  distinguishing feature and is not started. A deployment coming from Jasper
  re-authors its reports.
- **No offline viewing of results.** The service worker caches the application
  shell and no API response, deliberately: a cache keyed by URL serves one
  principal's data to the next person on that browser.
- **No changing your own email address**, and no API for querying the audit log —
  it goes to the log sink, where an operator's existing tooling reads it.
- **SAML is not implemented.** The seam it would use exists; OIDC is what is
  built.

### Upgrading

This is the first release, so there is nothing to upgrade from. What a later one
will have to tell you, and what already holds:

- **A rollout is safe and one-way.** Instances already running keep serving
  completely after a newer one migrates — reads, publishes and sign-ins —
  because migrations only ever add tables. But an old build refuses to open a
  database a newer one has migrated, so an instance that restarts mid-rollout
  does not come back. Take the backup immediately before deploying; a rollback
  is a restore.
- **The signing key is not recoverable.** Losing it ends every session, stops
  every share link and invalidates every embed token a host application holds.
- **Definitions this build will not accept are skipped, not fatal.** A stored
  definition that fails validation is logged at error and counted by
  `cronos_definitions_refused`; a schedule that will not arm by
  `cronos_schedules_unarmed`; a datasource that will not open by
  `cronos_datasources_unavailable`. The deployment starts and serves the rest,
  and the bad one can be deleted through the API. All three should be zero.
