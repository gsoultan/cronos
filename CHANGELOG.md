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

Versions are `MAJOR.MINOR.PATCH`. Up to 1.0 the minor was the roadmap release in
[docs/product.md](docs/product.md), and a minor could change behaviour a
deployment depended on. From 1.0 that stops: a breaking change to the API, the
definition format or a configuration variable takes a major, and anything that
needs a deployment to act says so under **Upgrading**.

---

## Unreleased

**"No such schedule" no longer means "this replica has no scheduler".** Firing a
schedule or resuming a run answered 404 for three different situations: a name
nobody has, another tenant's project, and an instance where `CRONOS_SCHEDULER`
was never set. The last one is a schedule the caller can see in `/v1/catalog`
being told it does not exist, and the fix it sends them looking for — a typo —
is not the fix. Those two endpoints now answer **503** with the variable to set,
and keep the 404 for the cases where refusing to confirm a name is the point.
Nothing is disclosed by the change: the caller has already resolved the project,
and `cronos_scheduler_armed` says the same thing.

---

## v1.0.0 — 2026-09-06

The roadmap's v1.0 — *Migrate* — cut as a release. Everything it named has been
in the tree for a while: the JasperReports importer, the HA scheduler with
leader election, SSO and audit in `ee/`, and the deployment documentation. What
this tag adds is the part that was still resting on trust.

**Releases now come from the tag rather than from a laptop**, with an SBOM per
archive and a signature over the checksums — see *Verifying a download* in
[docs/deploying.md](docs/deploying.md). Before this, an archive was built on a
machine nobody else could inspect.

**Three defects that were live in v0.5.0** are fixed below, one of which needs
you to check your schedules. And nine packages that had no test file have one,
including the executor every query in the product passes through.

The version means the format and the API are now under semver, not that the
product is finished. `docs/product.md` still lists what is unproven: the
importer has not met a four-hundred-file estate, and there are no design
partners yet. Those are commercial questions and this tag does not answer them.

**A slow scheduled run can be diagnosed.**
`cronos_stage_duration_seconds{stage="render"|"deliver"}` is a new histogram
covering the two halves of a burst. Nothing counted them before: a burst is
started by the scheduler, so the request histogram sees none of it, and "last
night took four hours" had one number and three candidates behind it. The two
stages are separated because their fixes are in different places — rendering is
your machine and the typesetter, delivery is somebody else's server. See "Where
a slow burst went" in [docs/deploying.md](docs/deploying.md). No new
dependency, and not distributed tracing: a burst calls no other service.

**Every API response carries `X-Content-Type-Options: nosniff`.** The bodies
here are built from a customer's own rows, and one that happens to begin like
markup should not be something a browser is free to sniff into HTML. The
proxy configuration under "Terminating TLS" now also sets the portal's headers,
including `Content-Security-Policy: frame-ancestors 'none'` — cronos has no
iframe path by design, and nothing was applying that decision to the page with
the publish button on it.

**Releases are built, catalogued and signed by CI rather than by a laptop.**
Pushing a `v*` tag now runs `.github/workflows/release.yml`: it cross-compiles
the archives, writes an SPDX SBOM for each from the binaries that actually
shipped, signs `SHA256SUMS` — which covers the archives and the SBOMs — and
publishes the lot as a GitHub Release. Signing is keyless, so there is no key
in this repository and none for you to fetch: what is attested is the release
workflow of this repository at that tag, and `cosign verify-blob
--certificate-identity` is how a download is checked. See "Verifying a
download" in [docs/deploying.md](docs/deploying.md).

Nothing about running cronos changes. `make dist` still produces the same
archives and now writes an SBOM beside each where `syft` is installed; the
release workflow sets `REQUIRE_SBOM=1` so a release without one fails rather
than ships. The container image is still `make image` and is still published by
hand — there is no registry here, which is a decision rather than an omission.

**The SSO state cookie is `Secure` behind a terminating proxy.** It was decided
from `r.TLS`, which is nil on every request that reached cronos through a proxy
that terminates TLS — the arrangement this documentation recommends. The cookie
therefore went out without `Secure` in exactly that deployment, and a browser
will send such a cookie over `http://`. It now follows `CRONOS_BEHIND_PROXY`,
which the rate limiter has always read. **If you run behind a proxy and have
not set `CRONOS_BEHIND_PROXY=1`, set it** — see "Terminating TLS" in
[docs/deploying.md](docs/deploying.md), which is new and has a working nginx
and Caddy configuration, including the `X-Forwarded-For` handling that has to
overwrite rather than append.

**A result set of exactly `maxRows` was refused as "more than `maxRows`
rows".** The row cap declined the limit's own last row instead of looking for
one beyond it, so a datasource capped at a million refused a million and the
hundred-thousand-row spreadsheet export refused the hundred thousand it exists
to allow. Only the boundary was affected; a set under the cap always worked and
one genuinely over it was always refused. Found by the first test the executor
has ever had.

**A schedule naming a channel the deployment has not got is refused at
publish.** It was accepted: `Validate()` asked only that `via` was non-empty,
and the channel was resolved for the first time in the burst, where a missing
one is fatal and the hour is 06:00. Publishing now checks the name against what
the deployment actually configured and says what it has. A deployment with no
channels at all refuses a schedule that delivers, rather than accepting all of
them.

**The portal's schedule form never sent the channel you picked.** Every
schedule it published went out as `email` whatever the picker said, and editing
an existing `via: file` or `via: s3` schedule silently rewrote it to `email` on
save. **Check any schedule that was created or edited through the portal and
was meant to deliver somewhere other than email.** The picker now offers only
the channels the deployment reports, and submits the one chosen.

**Telegram was never a channel, and three places said it was.** The v0.5.0
notes above, `docs/report-format.md`, and the schedule form all described or
offered it; nothing delivers through it. The claims are withdrawn. The portal's
Settings → Channels panel still renders a Telegram section against a fixture —
it is inert, and whether to build the channel or remove the panel is open.

**Tests where there were none.** Nine packages had no test file:
`internal/adapter/driver/sql` — the executor every query in the product passes
through, which is where the row-cap bug above was found — plus
`internal/platform/config`, `internal/app/share`, `internal/core/share`,
`internal/app/send`, `internal/core/document`, `internal/adapter/deliver/file`,
`internal/adapter/audit` and `ee/audit`. Nothing changed for a deployment
except the row cap; the rest is regression cover for behaviour that was already
correct, including the cross-tenant refusals on share links and the path
handling that keeps a customer's name out of a delivery path.

**The signing key can be rotated without an outage.**
`CRONOS_SIGNING_KEY_PREVIOUS` is a comma-separated list of keys that are
verified against and never minted with. Rotation is now: put the new key in
`CRONOS_SIGNING_KEY`, the old one in `CRONOS_SIGNING_KEY_PREVIOUS`, wait 24
hours — the ceiling on any token cronos issues — and unset it. Previously
replacing the key invalidated every session, every share link and every embed
token a host application held, at the instant it took effect, so in practice
nobody rotated. Nothing changes for a deployment that does not set the new
variable. A retired key under 32 bytes is refused at boot rather than ignored.

---

## v0.5.0 — 2026-08-17

The first tagged release. Everything through the roadmap's v0.5 is here: the
engine, paginated documents, the scheduler and bursting, the embed surface and
its tokens, and the authoring portal. Most of v1.0 is here too — leader
election, SSO, audit, and the deployment documentation — with one exception,
below.

**The engine.** YAML definitions with bound parameters and structural row-level
security, over Postgres, MySQL, SQL Server, SQLite and object stores — MySQL was
accepted by every layer and unreachable until this release, for want of one
blank import. Federation across engines behind `-tags duckdb`: one query joining
a customer list in one database to invoices in another, every source attached
read-only. Results are
bounded everywhere and streamed almost everywhere — a render never holds more
than a page, whether the table has fifty rows or two million.

**Documents.** Paginated PDF through Typst, with grouping, page breaks,
subtotals and running headers. Spreadsheet and CSV output. A hundred thousand
rows is the export ceiling and costs about 32MB while the workbook is written.

**Delivery.** A cron scheduler with per-project isolation and leader election,
so every replica can be armed and exactly one fires. Bursting renders one
document per recipient — 560–650 PDFs per second on the machine in
[docs/deploying.md](docs/deploying.md). Email and object-store
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

**Migration.** `cronos-import` turns a directory of JasperReports `.jrxml`
files into cronos definitions — one Dataset and one Report each, with identical
queries shared rather than copied. It writes nothing without `-out`, prints a
per-file work list graded blocked/review/note, and exits non-zero while anything
is blocked. It ships in both release channels beside `cronosd` — the Linux
archives and the container image — because the person holding four hundred
`.jrxml` files is as likely to be on a host as in a container.
`examples/jasper/` holds a report to try it on.

**Release archives.** `make dist` cross-compiles `linux/amd64`, `linux/arm64`
and `darwin/arm64` tarballs with a `SHA256SUMS` beside them, so a deployment
that already has systemd and a backup story does not have to adopt a container
runtime to run cronos. Two archives per platform: the community binaries under
BSL, and `cronosd-ee` under its own license, because the boundary
`scripts/check-license-boundary.sh` enforces in the build graph has to hold in
distribution too.

**Installing from an archive: install `typst` too.** It is not in the tarball —
a host takes it from the typesetter's own releases the way it takes any other
system dependency — and it is the one missing piece that says nothing. Nothing
outside the renderer looks for it, so every path except paginated output works
without it and a PDF schedule fails at six on the first of the month and nowhere
earlier. Run `typst --version` on the host before anything is scheduled. The
container image already carries it, and `make image` and CI both prove it does.
The portal is not in the archive either: it is static files on their own origin,
which is why `CRONOS_PORTAL_URL` is a URL. See
[docs/deploying.md](docs/deploying.md) — A host install.

### Not in this release

- **The `.jrxml` importer stops at the common 80%, and names the rest.**
  Subreports, crosstabs, images, conditional elements and columns computed by a
  Java expression do not come across; each is reported against the file that had
  it, so the remainder is a work list rather than a surprise. Two constructs are
  refused rather than imported wrong: a `$P!{}` parameter spliced into the SQL
  as text, and a query that is not SQL.
- **An imported dataset has no row-level security.** A `.jrxml` has none to carry —
  JasperReports Server enforced access above the report — so an imported
  dataset returns every row its query selects until somebody adds a predicate.
  The importer says so once per file. Do it before publishing anything an end
  customer can reach: [docs/migrating-from-jasper.md](docs/migrating-from-jasper.md).
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
