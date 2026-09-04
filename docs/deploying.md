# Deploying cronos

One process serves one project. `CRONOS_ORG` and `CRONOS_PROJECT` pin it, and
every token it accepts is checked against them. The definition store is
multi-tenant — every statement is scoped by organization and project taken from
the caller's identity — but the process is not, so a deployment serving several
projects runs several processes against one database. That is a deliberate
shape: the blast radius of a bad definition, a runaway query or a leaked
signing key is one customer's project.

## The image

```bash
make image                     # builds, and proves the typesetter is in it
docker build -t cronos:1.0 --build-arg CRONOS_VERSION="$(git describe --tags --always --dirty)" .
```

**Pass `CRONOS_VERSION`.** `go build` stamps the commit into a binary by itself,
but only when `.git` is present, and `.dockerignore` excludes it on purpose — it
is a large directory that invalidates a layer on every commit. Without the build
arg the image answers `unknown`, which is the one answer nobody wants from the
copy of cronos running in somebody else's cluster. CI passes it and then refuses
an `unknown` in the result, because an image that cannot say what it is still
builds and still serves.

The image needs no writable filesystem to start and serve, which the CI job
checks by running it `--read-only`. Paginated output is the exception: the
typesetter works in a directory under `/tmp`, so a read-only container wants
`--tmpfs /tmp` beside it — the same thing `PrivateTmp=true` does for the systemd
unit under A host install.

`cronosd -version` prints it, the startup log carries it as `build=`, and
`cronos_build_info{version="…"}` is a constant labelled with it — so "how much of
the fleet is on the new one" is a query, which during a rolling deploy is the
question.

Three stages: the portal's static files, the server binaries, and `typst`.
That last one is the only thing an image can be missing without saying so —
every path except paginated output works without it, so a PDF schedule fails
at six in the morning on the first of the month and nowhere earlier. Both
`make image` and CI run `typst --version` inside the result for exactly that
reason.

The image is CGO-free and runs as an unprivileged user. A deployment that
federates across databases needs `-tags duckdb`, which needs cgo, and builds
its own image from a base with the runtime.

Federation is one query across two engines — a customer list from the CRM's
Postgres joined to invoices somewhere else, with no ETL between them. Every
source is attached `READ_ONLY`, and that is checked against DuckDB rather than
against the string cronos generates: a delete through a mounted Postgres is
refused and the row is still there afterwards. cronos reads other people's
production databases, so a federation that can write to one is a different
product with a different risk. DuckDB downloads its `postgres` and `sqlite`
extensions on first use, which is why the CI job for it has a network and the
rest of the suite does not.

`TZ` is set and `tzdata` is installed, because "the first of the month at six"
is a local claim. A container with no zoneinfo resolves every timezone to UTC,
which is a statement dated an hour early in the wrong month.

Being a local claim, it meets the two days a year a clock is not a clock. When
the clocks go back, the hour happens twice — a schedule whose time falls inside
it used to fire in both, so everybody on the list was sent a second copy an
hour after the first. It now fires once, the same rule Vixie cron has had for
decades, while a schedule on a cadence still fires at every occurrence, because
a 25-hour day has 25 hours in it. When the clocks go forward, the missing hour
takes any firing inside it: that run is a day late, and the run that follows
covers both days and is labelled with both dates rather than pretending to be
one. Pick a time outside 01:00–03:00 local and neither applies to you.

## A host install

The other channel. A deployment that already has systemd, a package mirror and a
backup story wants binaries rather than a runtime to adopt, and `make dist`
produces them — see Cutting a release for what is in each archive and why there
are two.

```bash
sha256sum --ignore-missing -c SHA256SUMS   # the archive, before it is opened
tar xzf cronos_v1.0_linux_amd64.tar.gz
install -m 0755 cronos_v1.0_linux_amd64/cronos* /usr/local/bin/
cronosd -version
```

The binaries are `CGO_ENABLED=0`, so they do not depend on the host's libc
version and a release built anywhere runs here. They carry their own copy of the
timezone database (`time/tzdata`), so "the first of the month at six" resolves
without `tzdata` installed — though Go reads the system database first where
there is one, which is what a host that updates for a DST law change should do.
Federation is absent for the same reason it is absent from the image: DuckDB
needs cgo, so it is a build tag and a deployment that federates builds its own.

**Install `typst` as well, and prove it.** It is not in the archive — a host
install takes it from [the typesetter's own
releases](https://github.com/typst/typst/releases), the way it takes any other
system dependency — and it is the one thing whose absence says nothing. Nothing
outside the renderer looks for it: readiness does not, startup does not, and
every path except paginated output works without it. So a missing `typst` is a
PDF schedule failing at six in the morning on the first of the month and nowhere
earlier, which is exactly the failure `make image` and CI run `typst --version`
inside the container to prevent. Do the same here:

```bash
typst --version                            # before anything is scheduled, not after
```

**The portal is not in the archive either**, and `cronosd` does not serve it:
it is static files on their own origin in every real deployment, which is why
`CRONOS_PORTAL_URL` is a URL rather than a directory. Build it with `bun run
build` in `apps/portal` and serve `dist/` from whatever already serves static
files. The image bundles a copy only because a single-container demo has nowhere
else to put one.

A unit to start from. The environment goes in a file rather than the unit,
because `CRONOS_SIGNING_KEY` in a unit is a secret in a world-readable file:

```ini
[Unit]
Description=cronos
# Wants as well as After: After alone orders against a target nothing pulled in,
# so it is reached immediately and the unit starts before there is a route.
Wants=network-online.target
After=network-online.target

[Service]
User=cronos
EnvironmentFile=/etc/cronos/env
ExecStart=/usr/local/bin/cronosd
Restart=on-failure

ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
NoNewPrivileges=true
# ProtectSystem=strict makes everything read-only, and cronosd writes in three
# places. Name the ones this deployment uses, or publishing fails at the point
# somebody saves a definition:
#
#   the definitions directory, on every publish, if CRONOS_DEFINITIONS is the
#   store rather than a bootstrap — a publish writes the file and a copy under
#   .versions/
#   the store file, if CRONOS_STORE_DSN is SQLite rather than Postgres
#   the output directory of any `via: file` delivery
#
# Renders need none of it: the typesetter works in its own directory under /tmp,
# which PrivateTmp already provides.
ReadWritePaths=/var/lib/cronos

[Install]
WantedBy=multi-user.target
```

The cleanest deployment needs no `ReadWritePaths` at all — a Postgres store, and
delivery by email or object storage, leaves nothing on local disk to write. That
is worth aiming at for a second reason: it is also the deployment that survives
losing the host, which Backing up is about.

`cronosd` listens on `:8787` unless `CRONOS_ADDR` says otherwise, and answers
`/v1/ready` — point the load balancer at that rather than at liveness, and see
Probes for the difference. What has to be in `/etc/cronos/env` is the next
section.

`cronos-import` is in the archive too, so an estate migrating off JasperReports
does not need a container to do it — see
[migrating-from-jasper.md](migrating-from-jasper.md).

## What has to be set

| Variable | Why |
|---|---|
| `CRONOS_SIGNING_KEY` | 32 bytes minimum. Every token is signed with it. Rotate it through `CRONOS_SIGNING_KEY_PREVIOUS` rather than replacing it outright — see Rotating the signing key. |
| `CRONOS_ORG`, `CRONOS_PROJECT` | The one project this process serves. |
| `CRONOS_STORE_DSN` | The definition store. Without it, definitions are files and there is no sign-in, no run history and no sharing. |
| `CRONOS_ORIGINS` | The host applications allowed to call the API. Never `*` — the API reads an Authorization header. |

And what should be:

| Variable | Why |
|---|---|
| `CRONOS_SECRETS_DIR` | A directory of files, one per secret. Preferred over the environment: an environment variable is visible in `/proc` to anything running as the same user and appears in a crash dump. |
| `CRONOS_HISTORY_RETENTION` | e.g. `2160h`. Unset keeps run history for ever. |
| `CRONOS_BEHIND_PROXY=1` | Required behind a terminating proxy, and only there — see Terminating TLS. It keys rate limits on `X-Forwarded-For` and makes the SSO state cookie `Secure`. Believing it without a proxy keys every limit by a value the caller chooses; omitting it behind one sends that cookie without `Secure`. |
| `CRONOS_SIGNING_KEY_PREVIOUS` | Comma-separated keys accepted but never minted with, for a rotation. Unset outside one. |
| `CRONOS_AUDIT` | `log` by default, `off` to stop it. |
| `CRONOS_SMTP_HOST`, `CRONOS_SMTP_FROM` | A mail relay. Needed to deliver a schedule by email, and to invite anybody. |
| `CRONOS_PORTAL_URL` | Where the portal is served, for links in email. Without it, invitations are not offered — there would be nothing to put in the link. |
| `CRONOS_SCHEDULER=1` | Arms schedules. Off by default. Safe to set on every replica — see Running several. |
| `CRONOS_SCHEDULER_TICK` | e.g. `10s`. How often armed schedules are checked; a minute by default, which is cron's own resolution. Lower it if "06:00" has to mean 06:00 rather than some time in the minute after. |
| `CRONOS_METRICS_ADDR` | e.g. `127.0.0.1:9090`. Serves the exposition there and nowhere else — see Probes. |

## Terminating TLS

`cronosd` speaks plain HTTP and has no TLS flag. That is deliberate — a
certificate is a renewal, a reload and a private file, and every deployment
already has something that does all three. Put that something in front.

Two things have to be true, and the second is the one that gets missed.

**Bind cronosd where only the proxy can reach it.** `CRONOS_ADDR=127.0.0.1:8787`
on a host install, or an internal network on a container one. A process serving
plaintext on a public interface is one a client can reach directly, and every
protection below is then optional from the caller's side.

**Set `CRONOS_BEHIND_PROXY=1`.** It is not only about rate limiting. cronos
cannot see the browser's connection — behind a terminating proxy every request
arrives over HTTP, so `r.TLS` is nil and the process has no way to know the
deployment is served over TLS. This variable is how it is told, and two things
read it: the rate limiter keys on `X-Forwarded-For`, and the SSO state cookie
goes out `Secure`. Leave it unset behind a proxy and that cookie is one the
browser will also send over `http://`.

Set it on a proxy that does *not* terminate TLS and you get the opposite
problem, with a symptom worth recognising: the cookie is `Secure`, the browser
declines to send it back over `http://`, and every SSO sign-in fails at the
callback with "this sign-in did not start here". `CRONOS_BEHIND_PROXY=1` means
*there is TLS in front*, not merely *there is a proxy in front*.

Nginx:

```nginx
server {
    listen 443 ssl;
    server_name reports.example.com;

    ssl_certificate     /etc/letsencrypt/live/reports.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/reports.example.com/privkey.pem;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_set_header Host              $host;
        # set, not add. proxy_add_x_forwarded_for appends to whatever the
        # caller sent, and the limiter reads the first value — so a client
        # sending its own X-Forwarded-For picks its own rate-limit bucket.
        proxy_set_header X-Forwarded-For   $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Request-Id      $request_id;

        # A render of a large report is not a slow client.
        proxy_read_timeout 300s;
        # An export is streamed. Buffering it puts the whole file on the
        # proxy's disk before the browser sees a byte.
        proxy_buffering off;
    }
}

# Anything arriving on 80 is redirected, not served.
server {
    listen 80;
    server_name reports.example.com;
    return 308 https://$host$request_uri;
}
```

Caddy, which gets the certificate itself:

```caddy
reports.example.com {
    header Strict-Transport-Security "max-age=31536000; includeSubDomains"
    reverse_proxy 127.0.0.1:8787 {
        header_up X-Forwarded-For {remote_host}
        flush_interval -1
    }
}
```

`X-Forwarded-For` is the one header where the proxy has to overwrite rather
than append. Everything cronos trusts from it, it trusts because the proxy is
the last one to write it — see the rate-limit note under What has to be set.

**`CRONOS_ORIGINS` names the scheme.** `https://app.example.com`, not
`app.example.com` and not the `http://` the host application used in
development. An origin that does not match exactly is a CORS refusal that looks
like an outage.

## Probes

- `/v1/health` — liveness. Unconditional: a liveness probe that fails because
  a database is unreachable restarts a healthy process and does not fix the
  database.
- `/v1/ready` — readiness, which asks. 503 when the store is unreachable or at
  a schema this build does not know; 200 and `degraded` when a datasource is
  down, because the reports that do not read it still work.
What survives an outage of cronos's own database is worth knowing before one
happens: **reports keep rendering.** Definitions are held in memory and the
warehouse is a different database entirely, so a store outage takes out sign-in,
the run history and publishing, and leaves every reader — including an ISV's
embedded end customers — reading reports as normal. Readiness says 503 so an
instance can be routed around; liveness stays 200 so nothing restarts a process
that is perfectly healthy; and when the store comes back it recovers by itself
in about a second, with no restart. `scripts/live-failover.sh` stops a real
Postgres and starts it again to keep all of that true.

- `/v1/metrics` — Prometheus exposition. On the API listener by default, which
  means anybody who can reach the API can read it. It carries no customer data
  and no report names, but it is a running commentary on the business: which
  routes are used and how often, how many scheduled runs failed, how many
  deliveries did not arrive. Set `CRONOS_METRICS_ADDR` to serve it on an address
  of its own — `127.0.0.1:9090`, or a private interface — and it comes off the
  public listener entirely. Both `/v1/metrics` and `/metrics` answer there, so
  moving it is one variable rather than a change to every scrape config.

  Not the admin key, deliberately: that credential can publish definitions, and
  a Prometheus scraper should not hold one.

  `cronos_goroutines` is the one to alert on. A Go service fails in production
  by leaking goroutines — something is started per request, per run, per burst
  recipient, and one path forgets to finish — and cronos has the shape that
  hides one, because a burst starts a goroutine per recipient and a run is
  deliberately detached from the loop's context so a shutdown cannot cancel it.
  It should be flat at rest and return to flat after a burst; a count that
  climbs across days is a leak, and it is the only symptom until the process
  dies of memory nobody allocated on purpose. `cronos_heap_bytes` beside
  `cronos_heap_reserved_bytes` tells a leak from a fragmented heap, which is
  the question actually being asked at three in the morning. The container
  runtime knows the RSS of the container; it does not know which of the things
  inside it grew.

### Alert on the scheduler stopping

The first three of these are about *absence*, and they are the ones with no
substitute. Every other alert here counts something that happened — runs,
deliveries, failures — and a scheduler that is not running produces none of
them. Zero failures is what a perfect night looks like and also what a dead loop
looks like, so no alert written against a counter can tell them apart.

The process stays healthy the whole time. `/v1/health` is 200, `/v1/ready` is
ok, every request is served. The first person to notice is a customer who did
not get an invoice.

```promql
# 1. Nobody is scheduling. Across the fleet, no instance is armed —
#    CRONOS_SCHEDULER is off everywhere, which is its default.
max(cronos_scheduler_armed) == 0

# 2. Armed, and the loop has stopped going round. Above a few times
#    CRONOS_SCHEDULER_TICK, which is a minute unless you lowered it.
cronos_scheduler_seconds_since_tick > 180

# 3. Going round, and behind: something was due and has not run. Small values
#    are ordinary — a run takes time. Growing ones are a burst that cannot
#    finish inside its own interval.
cronos_schedule_overdue_seconds > 900
```

Then the ones that count what happened, in order of how much they cost when
missed:

1. `cronos_deliveries_total{result="failed"}` rising. Somebody's invoice did
   not arrive, and nothing else will tell you.
2. `cronos_runs_total{result="failed"}` — a schedule that ran and did not work.
3. `/v1/ready` returning `degraded` for longer than a deploy takes.
4. `cronos_request_duration_seconds` at the 99th percentile crossing whatever
   your customers' patience is.

## Running several

Set `CRONOS_SCHEDULER=1` on every replica. They elect one leader per project
through a Postgres advisory lock, and only the leader fires; the others arm
nothing and wait. If the leader goes away — a deploy, a crash, a node — another
takes over on its next tick.

This used to be a rule you held in your head: on for exactly one instance. Set
it twice and every customer got two copies of their statement; forget it and
nobody got one. Both are quiet, because the only party who notices either is the
recipient.

```promql
# Nobody is leading. The one alert that matters here — with several replicas
# armed, this should never be true for longer than a hand-over.
max(cronos_scheduler_leader) == 0
```

The lock is held on a session, so it is released when that session ends —
including when the process holding it is killed, because the socket closes and
Postgres notices. There is no lease to expire and no clock to agree on, and two
instances cannot both hold it.

What it does not give is instant failover in every case. A host that freezes
without closing its sockets keeps the lock until Postgres' keepalives give up,
which is minutes. That is the safe direction — nobody schedules for a while
rather than two instances scheduling at once — and the gauge above makes the gap
visible while it lasts.

On SQLite there is no election and none is needed: that deployment is one
process by construction, and it leads unconditionally.

## Stopping

On `SIGTERM`, in this order:

1. The listener stops accepting, and requests already in flight finish. Up to
   20 seconds.
2. The schedulers stop starting new runs. A burst already delivering keeps
   going, with its own 20-second grace, and shutdown waits up to 25 seconds for
   it.
3. The process exits.

So allow at least 50 seconds before anything sends `SIGKILL`. Kubernetes
defaults to 30, which cuts the burst:

```yaml
spec:
  terminationGracePeriodSeconds: 60
```

The order matters and is deliberate. HTTP drains first because a scheduled run
may call this deployment's own API; stopping the schedulers first would fail the
run the drain exists to let finish.

What this buys is worth stating plainly, because the failure it prevents is
quiet. A burst is one document per recipient. Killed halfway, half a customer
list has a statement and half does not — and the run record, which is the only
thing that could say which half, is written at the end.
`scripts/live-drain.sh` interrupts a burst of eight hundred and counts what
arrived; two defects lived under it that no unit test could see.

A burst too large to finish inside the grace is cut, and the run record says so.
If that happens often the schedule is too big for one instance, rather than the
grace being too short — raising it past the orchestrator's patience only adds
confusion before the same `SIGKILL`.

### Repairing a burst that stopped halfway

Whatever cut it — the grace, a warehouse, a mail relay refusing for ten minutes
— the run is recorded as `partial` and names how many were reached. Resume it:

```
POST /v1/runs/{id}/resume
```

It sends the same period's documents to whoever does not have one, and to
nobody else. The skip is read from every attempt at that period rather than from
the run you point at, so a burst that was cut, resumed and cut again can be
resumed a third time without anybody receiving two.

Only the deliveries that worked count as done: a failed one is exactly what a
resume exists to retry. Rows nobody needs are skipped before rendering, so
resuming eight hundred where twenty are outstanding typesets twenty documents.

Available on any replica with run history, not only the one that schedules —
during an incident, hunting for the leader is the last thing anybody needs.

## What one instance does

`scripts/load.sh` generates a corpus, stands a cronosd up against a real
Postgres, and measures. These are four runs of it, and the ranges are the point:

| In flight | p50 | p99 | requests/s | in use |
|---|---|---|---|---|
| 1 | 4.0–4.2 ms | 6.9–10.4 ms | 227–237 | — |
| 4 | 4.6–5.5 ms | 11.1–17.7 ms | 621–834 | 2–3 |
| 16 | 9.3–10.7 ms | 33.6–40.5 ms | 1,332–1,533 | 4–6 |
| 64 | 32.9–35.2 ms | 80.9–88.1 ms | 1,611–1,726 | 16 |

Bursting: **560–650 PDFs per second**, so five thousand statements is about
eight seconds of wall clock. Each is about 19 KB.

The report is the demo's `billing-summary` over a thousand customers and five
thousand invoices. The machine is a laptop with the database in a virtual
machine, which is why the ranges are two to three times wide, and why **you
should run this on your own hardware rather than size anything from this table**.
What it is good for is the shape:

- Throughput climbs steeply and then flattens between sixteen and sixty-four in
  flight, for about 12% more at four times the concurrency. The connection pool
  is the ceiling: in-use sits at 16, which is the limit, while p50 triples. So
  `maxOpen` on the datasource is the lever, not the instance count — and that is
  now a number a deployment can watch rather than one only this harness could
  see. `cronos_datasource_connections_in_use` against
  `cronos_datasource_connections_limit` answers it in production; in-use well
  below the limit means the ceiling is elsewhere and raising `maxOpen` will only
  open more connections on a database somebody else operates.
- The last row is the useful one for sizing. Nothing failed at any level, so the
  flattening is queueing rather than refusing.
- A burst is bounded by the typesetter, not by the warehouse: it is one render
  per recipient, and the query behind each is small.

One caveat about the harness itself, because it bit: the render limit is per
bearer, so load sent as one reader measures the rate limiter. An earlier run of
this table was 57% HTTP 429 and reported throughput five times higher than the
real figure, because a refusal is fast. `load.sh` mints sixty-four readers and
spreads requests across them, which is also the truer shape — one token
hammering is what the limiter exists to stop.

## Editing the same thing twice

Two people editing one report is a Monday. `GET /v1/definitions/{kind}/{name}`
returns an `ETag`; send it back as `If-Match` on the publish and a save built on
a version somebody else has already replaced is refused:

```
409  publish: changed since you read it: Report "billing-summary" is at
     sha256:85574a696982 and you started from sha256:a1d5787e332b —
     somebody else saved it
```

The portal's editors do this already. Without an `If-Match` the publish is
unconditional, which is what a deployment pipeline needs: it publishes from a
git repository that *is* the source of truth, and refusing it because the
running copy differs would refuse the deploy for doing its job.

What this closes is the window that matters — two people editing for ten
minutes — and not every window. Two saves landing milliseconds apart can still
both read the same current version and both succeed. Closing that needs a
conditional write in each store, which is worth doing when somebody has hit it.

## Dependencies

`govulncheck` and `bun audit` run in CI and fail a build. govulncheck traces
from this code's own call graph rather than listing everything a dependency ever
shipped, so what it reports is what an attacker could actually reach — which is
the difference between a gate people act on and a weekly list they stop reading.

It found six open against the standard library the first time it ran: the
toolchain was one patch behind and nothing was looking. `go.mod` names the patch
now, and the Dockerfile's builder names the same one, so the two cannot drift.

Two remain, and neither is reachable from this code: an out-of-bounds read in
`klauspost/compress/s2`, and `x/crypto/openpgp` being unmaintained. Both arrive
through the S3 client. They are reported and not called, so the gate stays
green; if either becomes reachable it will fail the build that makes it so.

Dependabot proposes the updates weekly, grouped rather than one pull request per
dependency — a stream of individual bumps is a stream nobody reviews, and an
unreviewed dependency bump is the risk it was meant to reduce.

## Cutting a release

```bash
RELEASE=v0.5.1 make release       # checks the tree and the changelog, then says what to run
git tag -a v0.5.1 -m v0.5.1
make dist                         # the Linux archives, stamped with the tag
make image                        # the container image, stamped the same way
```

**Two channels, and they carry the same commands.** The image is one way to run
cronos and not the only one: a deployment that already has systemd, a package
mirror and a backup story wants binaries rather than a runtime to adopt. So
`make dist` cross-compiles `linux/amd64`, `linux/arm64` and `darwin/arm64`
archives, and anything that ships has to be in both — a tool that exists in the
image and not the tarball is documentation that is wrong for half its readers.

**Two archives per platform, because there are two licenses.**

| Archive | Holds | License |
| :--- | :--- | :--- |
| `cronos_<v>_<os>_<arch>.tar.gz` | `cronosd`, `cronos-token`, `cronos-user`, `cronos-import` | BSL 1.1 |
| `cronos-ee_<v>_<os>_<arch>.tar.gz` | `cronosd-ee` | Commercial |

The split is not presentation. `scripts/check-license-boundary.sh` enforces in
the build graph that the community binary cannot reach `ee/`, and distribution
is the other half of that: one tarball holding both would put
commercially-licensed code inside the artifact somebody downloads expecting the
community edition. Each archive carries the LICENSE covering the binary next to
it, and `dist/SHA256SUMS` covers the archives so a download can be checked.

The binaries are `CGO_ENABLED=0`, so a release runs on a host that is not the
one that built it and does not depend on a glibc version. Federation needs cgo
and is a build tag, so it is not in a release archive for the same reason — a
deployment that wants DuckDB builds with `-tags duckdb`.

Neither archive holds the portal. It is static files served from its own origin
in every real deployment — see **What has to be set** — and the image bundles a
copy only because a single-container demo has nowhere else to put it. Nor does
either hold `typst`: paginated output shells out to it, and a host install gets
it from the typesetter's own releases.

`make release` refuses two things and only two, because both produce a version
nobody can act on: a dirty tree, which tags a build that cannot be rebuilt from
the tag, and a `CHANGELOG.md` with no entry for the version, which is a number
an operator can read off `cronosd -version` and then not look up.

`VERSION` in the Makefile is `git describe --tags --always --dirty`, so once a
tag exists every build downstream of it names that tag, and `make image` passes
it as the `CRONOS_VERSION` build arg. Untagged builds report the commit, which
is the right answer for a build from a branch.

CHANGELOG.md is written for the person running cronos, not the person who
changed it: what behaviour is different, what a deployment has to do about it,
and what breaks if they do nothing. Anything a reader cannot act on belongs in
the git history instead.

## Upgrading

Migrations are forward-only and run at startup, each in a transaction. A
database at a schema newer than the binary is refused rather than written to,
so a rollback needs the old data and a restore, not a downgrade.

Instances starting together is fine. On Postgres, one takes an advisory lock and
migrates while the rest wait — up to five minutes, which is a large migration on
a large table rather than a number worth tuning. Without it, four instances
against a fresh database left three unable to start, and not at some exotic
migration: `CREATE TABLE IF NOT EXISTS` is not concurrency-safe in Postgres, so
they collided on the very first statement.

### A rolling deploy

Different versions running together is the other half, and it is what a rollout
is for the length of one. `scripts/live-upgrade.sh` builds two binaries from two
commits and runs them side by side against one database; what it finds:

**Instances already running keep working, completely.** The old build serves
every route after a newer one has migrated — the catalogue, run history, the
roster, rendering, and publishing and signing people in, which is the half that
matters, because a rollout where the old instances can only read is a rollout
with a write outage in the middle of it. That is not luck. Migrations only ever
add tables, never a column to an existing one, so code that has never heard of
the new tables is unaffected. It is why they are written that way.

**The migration is the point of no return.** It happens on the first new
instance to start, and from that moment an old build will not open the database
— it refuses at startup rather than writing to a schema it does not understand,
and says which version it found. So an old instance that restarts mid-rollout
does not come back: a node eviction, an OOM, or a rollback. Take the backup
immediately before the deploy, not the night before; a rollback is a restore.

**Upgrading from a version before this one has a capacity cliff.** Builds up to
and including the one that added the tenancy table report readiness `503` when a
newer cronos has migrated. Readiness is what keeps an instance in the load
balancer, so the first new instance to migrate takes every old one out of
rotation at once, and the whole of the traffic lands on the single new pod for
the rest of the rollout. Nothing is wrong with those instances — they are
answering every request correctly, as above — but the probe says otherwise.
This release reports not-ready only when the schema is *behind* what the build
reads, which is a restore of an older dump underneath a running process and is
genuinely unservable. Upgrading past this version once fixes it for every
upgrade after. Until then: scale up before deploying, or accept the window.

Two versions against one database is ordinary during a deploy; what is not is an
old binary writing to a schema a new one has already changed, which is what the
refusal above prevents. So roll forward normally, and roll back by restoring.

## Backing up

Two things hold state, and they are not equally replaceable.

**The definition store.** Definitions, their version history, users, shares and
run history. Back it up the way you back up any Postgres — this schema has no
special requirement, and every table carries the tenant it belongs to. Restore
is a restore.

**The signing key.** Not in the database, and losing it is not recoverable:
every session ends, every share link stops opening, and every embed token a
host application holds becomes invalid. Keep it wherever you keep the thing
that would end your business if you lost it.

### Rotating the signing key

`CRONOS_SIGNING_KEY_PREVIOUS` is a comma-separated list of keys that are
verified against and never signed with, so a rotation is a rolling change
rather than an outage. Without it, replacing the key invalidates every token in
every host application at the instant it takes effect — which is an outage in
somebody else's product, caused by our housekeeping, and the reason a key that
cannot be rotated gently is a key that never gets rotated at all.

```bash
# 1. The new key signs; the old one is still honoured.
CRONOS_SIGNING_KEY="<new>"
CRONOS_SIGNING_KEY_PREVIOUS="<old>"
```

Roll that out. From the moment the last replica has it, everything minted is
signed by the new key and everything already in the wild still verifies.

```bash
# 2. After 24 hours — token.MaxLifetime, the longest any token can live.
CRONOS_SIGNING_KEY="<new>"
# CRONOS_SIGNING_KEY_PREVIOUS unset
```

Nothing signed by the old key can still be valid by then, because `Mint` would
not have issued it: 24 hours is the ceiling on every token cronos produces.
Waiting is what makes the second step safe, and skipping it is the outage the
first step existed to avoid.

Two things do not wait out. **Sessions and share links** are the tokens
that are held longest in practice, and they are bounded by the same 24 hours.
And a key retired here still has to be a real key — one under 32 bytes is
refused at boot rather than ignored, because a weak key on the way out is a
weak key.

The definitions directory is not state. It is a bootstrap: once the store holds
anything, the directory is not consulted again.

## Restoring, and proving you can

A backup nobody has restored is a hypothesis. The drill, in full:

```bash
# 1. Restore into an empty database.
psql "$RESTORE_DSN" < backup.sql

# 2. Point a cronosd at it with the real signing key, on a port nobody uses.
CRONOS_STORE_DSN="$RESTORE_DSN" CRONOS_ADDR=:9999 cronosd

# 3. It must reach ready, which proves the schema is the one this build knows.
curl -sf localhost:9999/v1/ready

# 4. And a report must render, which proves the definitions came back whole.
curl -sf -X POST localhost:9999/v1/reports/<a-real-report> \
  -H "authorization: Bearer $(cronos-token -audience portal -role viewer …)" \
  -d '{}'
```

Step 4 is the one that matters. A database that restores and a product that
serves are different claims, and only the second is the one anybody is asking
about.

`scripts/live-restore.sh` runs all four on every push, against a real Postgres:
it stands a deployment up, backs it up, **drops the original**, restores into an
empty schema, and renders the same report out of it — comparing the number, not
just the status code. A drill nobody runs is a drill that drifts, and what
drifts is the schema: a migration that adds a NOT NULL column with no default
breaks a restore and nothing else, because the forward path keeps working
perfectly and the failure waits for the day the backup is the only copy left.

## Adding people

Two ways, and the first is the one to use.

**An invitation.** `POST /v1/people` with an email, a name and a role and *no*
password. Nothing is created: a single-use secret is mailed to them, and the
account comes into being when they set a password only they have ever seen.
Needs `CRONOS_SMTP_HOST`, `CRONOS_SMTP_FROM` and `CRONOS_PORTAL_URL`; the portal
asks whether they are set and offers the choice only where they are.

**A password you choose.** The same endpoint with a password. The account exists
immediately and somebody other than its owner knows its credential — which then
lives in whatever carried it, a chat message or a ticket or a sent folder, known
to at least two people from the moment it exists. Kept because a deployment
without a mail relay has to be able to add its second administrator somehow, and
because the first one is created by `cronos-user` the same way.

The invitation's secret is 256 bits from `crypto/rand`, stored as a SHA-256 hash
and never in the clear, spent by the `UPDATE` that accepts it, and dead after a
week. It travels in the link's **fragment** — after the `#` — which a browser
sends to no server, so it is not in the portal's access log, its CDN's, or a
`Referer` header. `GET /v1/auth/invitation` answers the same for expired, spent
and never-issued, because that endpoint has no session by design and telling
them apart would say which addresses somebody is onboarding.

Rows are swept hourly: unusable ones as soon as they expire, accepted ones after
thirty days, by which point the audit log is the record and what is left here is
an address kept by accident.

```bash
./scripts/live-invite.sh
```

Runs a real SMTP server and a real database, invites somebody, reads the message
out of the mailbox, follows the link, and then checks the two things that matter
most: that the link does not work twice, and that replaying it changes nothing.

## Forgetting a password

Until recently there was no way back. `cronos-user` creates accounts and
deliberately will not reset one, so the answer to the commonest support request
in software was a shell on the server and a bcrypt hash written by hand — an
outage for the person, and a standing reason for somebody to keep a production
DSN on a laptop.

**Forgot your password?** on the sign-in page, where the deployment can send
mail. It is absent where it cannot: a link that turns into "no mail server is
configured" is a promise made to somebody who is already locked out, so
`/v1/auth/methods` says whether a reset is possible at all and the page asks
before offering.

Five things decide whether it is safe, and each is asserted by
`scripts/live-reset.sh`:

- **Asking says nothing.** The answer is identical for an address with an
  account, one without, a disabled account, five links already sent this hour,
  and a mail server that refused — the same words, and sent before any of the
  work, so the time taken carries nothing either. An endpoint that answered
  differently is a way to ask, one address at a time, who a customer's staff are.
- **The secret travels in the fragment**, not the query string. A browser sends
  a fragment to no server, so it reaches no proxy log, no access log and no
  `Referer` header — the same reasoning an invitation link already followed.
- **One use.** The link is spent by the UPDATE that redeems it, so two clicks
  arriving together produce one reset. Every other outstanding link for that
  account is spent at the same time: the second email is still in the mailbox,
  and "ask again, wait for the owner to reset, use the older link" is an attack
  rather than a hypothesis.
- **Every session ends**, in the same transaction as the password. "I cannot get
  in" and "somebody else is in" are the same sentence from outside, and a reset
  that leaves the intruder signed in has recovered nothing.
- **It is not a way around a second factor.** No session is handed back — signing
  in afterwards still asks for the code. A reset proves control of a mailbox,
  which is the exact thing a second factor exists because it is not enough.

An hour, not renewed, and asking again is free. Rows are pruned an hour after
they are finished with, by the same sweep that clears invitations.

## A definition this build will not accept

Two things decide what happens, and they used to be one.

**At publish**, a definition is validated and refused where the person who wrote
it can still see it — and a schedule is parsed by the same function that arms
it, rather than by a second rule kept in step with the first. That arrangement
is what produced the bug: the core is standard library only, so it could count
five fields in a cron expression and check that a timezone resolves, and the
parser that actually has to arm the schedule ran at startup. Everything the two
disagreed about published with a 200. `0 25 1 * *` is five fields, is hour
twenty-five, and is a plausible typo for a monthly job at six; `Europe/Berln`
was checked for being non-empty and never for existing. The
running instance carried on perfectly well — a schedule is parsed when the
process starts, not when it is stored — and then the next restart found a
schedule that would not arm and refused to start at all. Anybody who could edit
could take the whole deployment down with a typo, days in advance, and the
outage would land on a deploy that looked like it broke something.

The two fields that were reachable this way are closed. A datasource's DSN is
not checked at publish and deliberately so — a warehouse being unreachable is
not a reason to refuse a definition — but neither is it fatal: `sql.Open` is
lazy, so a bad one costs the reports that read it rather than the process. A
delivery channel nobody registered is the same shape, failing the run that uses
it, where the schedule's alert address is there to say so.

**At startup**, a stored definition this build will not accept is skipped rather
than fatal. Adoption is still all or nothing on the ordinary path; this is the
fallback when that refuses. It matters because validation gets stricter and the
store outlives any one build, so a definition published under laxer rules
becomes, one release later, a process that will not start — and with the API
down, the only way to remove it is a prompt on the database. That is the shape
of every unrecoverable failure: fixing the broken thing needs the broken thing.

A **datasource** this build cannot open is the same, and was the last one open.
`driver: mysql` was accepted by validation long before any MySQL driver was
registered, so an editor could publish one, get a 200, and take the deployment
down at its next start. MySQL is registered now — the dialect, the registry and
the federation mount had all been built, and the feature was a blank import away
from working — and a source this build still cannot open is skipped. Reports
that read it fail with a message naming it; reports that do not keep working.
The reason names the source and the driver and **not** the DSN, which used to be
printed on the reasoning that it says `${secret:…}` where the password goes:
true for a definition using a secret reference, false for an inline password,
which is allowed and is what a first deployment writes.

It is not quiet, which was the real fear behind refusing:

- every refusal is logged at error, naming the definition and why
- `cronos_definitions_refused` counts the ones not being served at all
- `cronos_schedules_unarmed` counts the ones stored that will never run
- `cronos_datasources_unavailable` counts the sources that would not open

All three should be zero. When they are not, the deployment is up, the other
definitions are being served, and the bad one can be deleted and republished
through the API. `scripts/live-typo.sh` drives exactly that, including the
repair.

## Ending sessions

A portal token is signed and stateless: nothing is written when one is minted,
so there is no list of sessions to show and no way to end one and keep another.
The account page used to offer "sign out everywhere else" over a list of devices
and cities that were invented in the browser — a control that did nothing beside
data that was not true.

What replaced it is one timestamp per account, in `cronos_sessions_cut`. Every
request already asks whether the account is still active; it now also asks when
its sessions were cut, and refuses any token minted before that. `POST
/v1/auth/sessions/end` draws the line and hands the calling browser a fresh
token dated at it — so the person who pressed the button stays signed in and
every other machine is signed out.

The line falls on the **next second boundary**, not on the current instant. A
token's `iat` has second granularity, so a line drawn part-way through a second
cannot be told from a session minted earlier in that same second — driving it
with two real browsers found exactly that, and rounding up removes the ambiguity
instead of narrowing it. The replacement token's `iat` is therefore up to a
second in the future, well inside the 30 seconds of skew every token is already
verified with.

It takes effect within about five seconds, which is how long the standing answer
is cached — the same trade already made for disabling somebody.

```bash
./scripts/live-sessions.sh
```

## Two-factor authentication

TOTP, RFC 6238, against any authenticator app. Enabled per account by its owner;
there is deliberately no path by which an administrator enrols, inspects or
removes somebody else's — enrolling for another person is meaningless, they hold
the phone, and removing one is exactly what a social-engineering call asks for.

The portal has shown an enrolment wizard since before there was a server to
enrol against. It accepted any six digits, the QR code beside it was noise with
finder squares drawn in the corners, and the recovery codes were generated in
the browser and stored nowhere. An account could finish that wizard and be
marked as protected by a secret that existed in no app anywhere — which is worse
than offering nothing, because its owner then picks a weaker password.

What is enforced now:

- **Enrolment is proved.** Nothing is switched on until a code computed from the
  stored secret comes back. `confirmed_at` is NULL until then and `Protected`
  reads it, so an abandoned enrolment protects nothing and is replaced by the
  next attempt.
- **A code is spent.** It is valid for a whole thirty-second step, long enough to
  be read off a shoulder or a screenshot. The step is recorded, and the check is
  inside the `UPDATE` — two sign-ins racing with one stolen code let one in.
- **The secret is write-once.** Readable while enrolment is in progress, so a
  reloaded page can show the QR again, and never afterwards. An endpoint that
  hands it back turns a stolen session into a permanent second factor of the
  attacker's own.
- **Turning it off takes a code**, or a recovery code. Without that, a stolen
  session strips the factor off the account it stole at the moment the factor is
  all that is left.
- **Recovery codes are passwords.** Ten, from Crockford's alphabet so nothing is
  misread, shown once at confirmation, stored as SHA-256 hashes, spent by the
  `DELETE` that checks them, and deleted with the factor.

Sign-in sends the password and the code together. A two-step exchange would need
a challenge that says "this password was right", and that challenge is a
credential to steal, expire, rate limit, and — worst — a way to learn which
accounts have a factor by watching which ones issue one. Instead, an attempt
with the password alone against a protected account answers `{"factorRequired":
true}`, which is only ever said to somebody who already proved the password.

The two steps have **separate rate limits**, which a live run forced: they shared
one, so three mistyped codes locked somebody out of their own password. The
password limit stays tight because password lists are long; the code limit is
looser because the legitimate retry is common and guessing six digits is
hopeless at any rate a person would tolerate.

```bash
./scripts/live-2fa.sh
```

Enrols against a running server and computes every code independently, in
Python, from the `otpauth://` URI — because a test that checks cronos against
cronos passes just as well when both halves are wrong, which is how the old
wizard shipped. It takes about forty seconds: most of that is waiting for the
sign-in rate limit to refill, which is the limiter working.

## Checking the portal itself

```bash
./scripts/live-portal-2fa.sh
```

Starts a server on a fresh database, starts the portal against it, and drives
two-factor enrolment through a real browser — computing every code in Node
rather than asking cronos what to type.

It exists because the type checker, the linter and the API checks all passed
while the enrolment wizard was rendering *inside* the account page, between the
password and the sessions, with two sets of Back/Continue on screen; and while
two sibling panels were keyed by the same counter, so React was quietly
discarding one. Neither is a thing any of those tools can see.

## The first run

A fresh install has a database, a server and no accounts. Until there was a
`/setup`, the only way to create the first one was `cronos-user` on the machine —
fine for somebody with shell access, impossible for anybody handed a URL.

Open the portal. It offers to set the deployment up, takes an email, a password
and the names of the first organisation and project, and signs you in as an
account that administers both the deployment and that project.

It is the most dangerous endpoint in the product, so:

- It is open only while **no account exists at all** — not "no administrator",
  no account, anywhere. The first success closes it and nothing reopens it short
  of emptying the users table.
- The check happens inside the write, and the write is one transaction in the
  database — a marker row with a fixed key, inserted alongside the account and
  the grant. Two people sent the same URL create one administrator; so do two
  cronos processes brought up against one empty database before anybody has the
  address, which a mutex in one process could not have managed. A losing request
  writes nothing at all, so the address it used is not consumed.
- An upgrade is not a first run. Deployments that predate the marker row have
  accounts and no marker, so the check counts them too — without that, every
  upgrade would offer its next visitor a deployment administrator.
- It needs a store. A file-backed deployment has nowhere to keep an account, so
  the page is not offered rather than offered and broken.
- There is no token in the log and no environment variable to unlock it. Both
  are things people leave switched on.

The names you type become identifiers — "Acme Logistics" is stored as
`acme-logistics` — because they are half of every tenancy check and part of a
path when definitions live on disk. A single-project process adopts the name it
is given at that moment; one told what to serve by `CRONOS_PROJECTS` keeps what
it was told, and says so in the log if a first run names something else.

```bash
./scripts/live-setup.sh
```

## Administering the deployment

A tier above organisations, for whoever runs the servers. **Settings →
Deployment**, visible only to an account that holds it.

It can list every tenant and every account, **create the first account of an
organisation that has none**, move somebody from one organisation to another,
turn access off anywhere, and grant or revoke itself.

Creating one names the organisation and project, which is the single thing no
project administrator may do — `/v1/people` always creates into the caller's own
project. The password is chosen rather than emailed: an invitation is addressed
to a project's own people by somebody who works with them, and this is a
deployment operator standing up a customer who has no account, no project and
nobody to invite them yet.

It **cannot read any project's data**. Opening a report, running a query or
seeing a dataset still requires membership in that project — `Principal.Platform`
is deliberately absent from `CanRead`, `CanEdit`, `CanAdminProject` and
`CanAdminOrg`, and there is no route under `/v1/platform` that returns a row of
anybody's warehouse. That is what makes a leaked platform credential a
control-plane problem rather than every customer's data at once. Support that
needs to see what a customer sees adds itself to that project, and the audit log
records that it did.

Somebody without the permission is answered **404**, not 403: a "you may not"
would confirm to anybody probing that the tier exists and that some account
holds it.

The last administrator cannot be revoked. A deployment with none cannot make
another over HTTP, because the endpoints that grant it require the permission
being granted. The way back is the command line:

```bash
cronos-user -dsn "$CRONOS_STORE_DSN" -email ops@acme.example -platform
```

It grants the permission to an account that already exists and asks for no
password: the account being rescued is somebody else's, and a command that reset
their password in order to grant a permission would lock its owner out to let
them back in. On a new account, `-platform` alongside the usual flags creates
and grants in one go. There is deliberately no `-revoke`: removing it is the
API's job, where the guard against taking the last one lives. Revoking anybody
else ends their sessions in the same transaction, because the permission travels
in the token and would otherwise outlive the revocation by up to eight hours.

Every cross-tenant action is audited under its own `platform.*` prefix, so "who
reached across tenants, and when" is a question the log answers directly.

## Requiring a second factor of everyone

**Settings → Security**, per project, by a project administrator.

The flag was never the hard part. What made this wait is that somebody with no
second factor cannot enrol without signing in and cannot sign in without
enrolling — so refusing the sign-in locks a team out of its own reporting on the
afternoon it is switched on, and puts an administrator on the phone being asked
to turn a second factor off. That is the exact call a second factor exists to
make suspicious.

So nobody is refused. Somebody without one signs in normally and gets a session
that reaches the enrolment routes and **nothing else** — every other route
answers 403, and the portal shows the wizard instead of a shell of refusals.
Finishing it hands back an ordinary session in the same response, so they are not
asked to sign in again thirty seconds after proving a password and a code.

The gate is an allow-list wrapped around the whole API rather than a check per
handler, and the direction matters: a route added tomorrow is refused to these
sessions until somebody deliberately lists it. A deny-list would let every new
route through and nobody would be looking.

Turning it on does not touch sessions that already exist, including the one that
turned it on — the requirement bites at the next sign-in. The panel shows how
many people have one before you switch it, because "12 people, 4 without" is the
whole decision.

```bash
./scripts/live-require-2fa.sh
```

## The live checks

`scripts/live-*.sh` drive a running cronos the way a person or a browser would.
Between them they have found around a dozen defects no unit test could: a QR code
that encoded nothing, a session cut that spared the phone it was meant to end, a
chart that compiled to `GROUP BY 1` against a database that reads the `1` as a
constant, an enrolment wizard rendering inside the account page, and one
organisation's report catalogue shown to another organisation's administrator
out of a browser cache that nothing ever emptied.

| Script | What it drives | Needs |
|---|---|---|
| `live-setup.sh` | first run, platform administration, the CLI recovery path | go |
| `live-sessions.sh` | ending every other session | go |
| `live-2fa.sh` | enrolment and sign-in, with codes computed in Python | go, python3 |
| `live-require-2fa.sh` | turning the requirement on, and the enrolment-only session | go, python3 |
| `live-invite.sh` | inviting somebody and reading the mail | a mail server |
| `live-drain.sh` | SIGTERM in the middle of a burst of eight hundred | go, typst |
| `live-disconnect.sh` | hanging up on a slow report, watched from `pg_stat_activity` | go, Postgres, psql |
| `live-scheduler-stalls.sh` | a scheduler that stops inside a process that stays healthy | go |
| `live-leader.sh` | three replicas all armed, one scheduling, and failover | go, typst, Postgres |
| `live-resume.sh` | resuming a partly delivered burst, without sending anybody two | go, typst, Postgres |
| `live-restore.sh` | the restore drill below, end to end | go, Postgres, pg_dump |
| `live-boundaries.sh` | row scope, audiences, tenancy, forged tokens, share links | go |
| `live-failover.sh` | the definition store going away and coming back | go, podman |
| `live-sso.sh` | a whole OIDC sign-in and single log-out | Keycloak |
| `live-sqlserver.sh` | a report against SQL Server | Azure SQL Edge |
| `live-mysql.sh` | a report against MySQL | MySQL 8 |
| `live-portal-2fa.sh` | the same enrolment, through a browser | bun, chrome |
| `live-handover.sh` | one browser, two people, two organisations | bun, chrome |
| `live-reset.sh` | forgetting a password and getting back in | a mail server, bun, chrome |
| `live-typo.sh` | a bad definition that used to stop the process starting | go |
| `live-upgrade.sh` | two versions against one database, through a migration | go, podman, git |

All of them run in CI on every push. Six share a job and take about a minute,
`live-disconnect.sh` runs beside the Go tests because it wants the same
Postgres; SQL Server, Keycloak and the browser suites have their own, because a
1.7 GB image or a two-minute realm setup should never be the reason a typo takes
ten minutes to fail.

Two of them ran nowhere but on the machine of whoever remembered, for a while,
which is how `live-sso.sh` came to be asserting things about a stale Python
server left behind by an earlier run. It now refuses to start if anything is
already listening on its ports — a check that quietly talks to the wrong server
is worse than one that stops.

A script that finds a server already listening uses it and leaves it alone, so
CI's service containers work without the script knowing it is CI. The exception
is `live-sqlserver.sh`, which needs telling: set `CRONOS_SQLSERVER_RUNNING=1`.
Without it the script starts its own Azure SQL Edge and removes it afterwards —
Edge because it is the SQL Server engine and the only one of the family that
runs on arm64, where `mssql/server:2022` is amd64 and segfaults under emulation.
Both paths are exercised: the ten assertions include `@p1` binding where the
compiler expects, `DATEADD`/`DATEDIFF` bucketing months the way a person would,
and a `DECIMAL` arriving as a number rather than as bytes.
Locally it starts Azure SQL Edge instead, because `mssql/server:2022` is amd64
and segfaults on an arm64 laptop under emulation — the same engine either way.

## Development

```bash
./scripts/dev.sh
```

The API on 8080 and the portal on 5173, **connected**, with accounts in
`.dev/cronos.db`. On a fresh clone the first thing the browser shows is the
first-run setup; after that it is the sign-in page. Delete `.dev/` to start over.

It did not used to connect. The script started both halves and never told the
portal where the API was, so it ran on sample data beside a server nobody was
talking to — and every part of cronos that needs an account was invisible in
development: signing in, signing out, setup, invitations, second factors, the
people list, the deployment tab. `CRONOS_ORIGINS` had always allowed the portal
to call the API; nothing ever told the portal to.

That is worth naming as the cause rather than a detail. It is how a two-factor
wizard that accepted any six digits and a device list invented in the browser
both survived: anybody running the development command saw the sample portal,
where neither was ever shown.

```bash
./scripts/dev.sh --samples
```

Sample data and no server, which is what the browser suites exercise and what
makes the interface workable before a server exists. Still available, no longer
the default.

```bash
bun run --cwd apps/portal walk
```

Walks every page of a connected portal, reporting console errors and any API
call that did not answer 2xx. The first time it ran it found that the Reports
page — the landing page of the product — had no live path at all.


