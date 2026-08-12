# cronos

A report engine and repository. Definitions are files, queries are governed,
and a report can be embedded in someone else's application.

Licensed under the [Business Source License 1.1](LICENSE) — internal
production use is unrestricted; offering cronos itself as a service to third
parties needs a commercial licence. It converts to Apache 2.0 on 2030-08-10.

## Run it

```bash
make setup          # verify the toolchain, install dependencies
make dev            # the API and the portal together
```

The demo serves `demo/definitions` over `demo/seed.sql`, so there is real data
behind the first report you open.

```bash
export CRONOS_SIGNING_KEY="development-key-at-least-32-bytes-long"
CRONOS_DEFINITIONS=demo/definitions CRONOS_SEED=demo/seed.sql go run ./cmd/cronosd

# What a host application's backend does, server to server:
TOKEN=$(go run ./cmd/cronos-token -scope customer_id=c-1 -report billing-summary)

curl -s -X POST localhost:8787/v1/embed/reports/billing-summary \
  -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' \
  -d '{"filters":{"period":{"op":"between","values":["2026-01-01","2026-12-31"]}}}'
```

Change `-scope customer_id=c-2` and the same report returns a different
customer's invoices. Drop `-scope` and it returns none — see
[docs/tenancy.md](docs/tenancy.md).

## Publishing definitions

Set `CRONOS_ADMIN_KEY` and the management API is mounted. Without it the server
is read-only and the endpoints do not exist — an endpoint that only ever says
no is one somebody will spend an afternoon probing.

```bash
curl -X POST localhost:8787/v1/definitions \
  -H "Authorization: Bearer $CRONOS_ADMIN_KEY" --data-binary @report.yaml
# {"kind":"Report","name":"billing-summary","version":"sha256:85574a696982"}
```

Publishing validates before it stores, and it goes as far as compiling every
block: a field the dataset does not publish, a filter bound to a column nobody
has, a report reading a dataset that does not exist. Each of those otherwise
fails identically at 6am in the middle of a burst.

The version is the document's content hash, so republishing unchanged bytes
returns the same one and a run record naming a version can be replayed against
exactly what produced it. Previous versions are kept under `.versions/`, and
deleting a definition does not remove them — a run that used it must still be
reproducible.

Changes are live immediately; there is no restart.

By default definitions are files, because a definitions directory is somebody's
git repository and a publish is a commit they can review. Set `CRONOS_STORE_DSN`
and they go to a database instead, which is what makes management
multi-tenant: every statement is scoped by organization and project taken from
the caller's identity, never from an argument. The file store holds one project
and refuses a principal acting anywhere else, rather than serving it to whoever
asks.

A password never appears in a definition. The format carries a reference —
`${secret:warehouse_password}` — and it is resolved where the connection is
opened, from a file in `CRONOS_SECRETS_DIR` if there is one and from
`CRONOS_SECRET_<NAME>` otherwise. Files first, because an environment variable
is visible in `/proc` to anything running as the same user and appears in a
crash dump. A reference nothing can supply stops the server at startup, naming
what was missing, rather than becoming a connection error at six in the
morning; and the resolved value exists between the resolver and `database/sql`
and nowhere else — not in the management API's copy, not in a log line.

## Operating it

Running it is [docs/deploying.md](docs/deploying.md): the image, what has to be
set, the probes, the alerts worth having, and a restore drill that ends by
rendering a report rather than by the database coming back.

`/v1/health` is liveness: this process is running, do not restart it.
Unconditional on purpose — a liveness probe that fails because a database is
unreachable restarts a healthy process and does not fix the database.

`/v1/ready` is readiness, and it asks. The store is required; a process that
cannot reach it cannot publish, sign anybody in or record a run. Datasources
are not: one warehouse of four being unreachable leaves three-quarters of the
reports working, so the answer is `degraded` and still 200 — taking the
instance out of rotation would fail those too. Probes are cached for a few
seconds, because a readiness check every second from each of several load
balancers, each opening a connection to every warehouse, is a denial of service
performed on your customers' databases.

`/v1/metrics` is Prometheus exposition: requests by route and status, how long
they took, and runs and deliveries by result — the last of which has no other
source, since a burst that failed at 06:00 is over by the time anybody looks.
Labelled by route and never by path: a path carries a report name, and one
series per report is one series per customer of your customer.

Run history grows without bound. `CRONOS_HISTORY_RETENTION=2160h` prunes daily;
unset keeps everything, because how long a business must be able to show what
it sent its customers is a legal question with a different answer in every
jurisdiction, and answering it on their behalf is not this product's job.

Every read, publish, delete, share and sign-in is recorded — who, in which
project, against what, and whether it was allowed. Refusals too: an audit that
only shows what succeeded answers the wrong half of every question. A read
records the row scope it ran under, because "somebody read a report" is not the
question anybody asks; "which customer's rows went to whom" is. It goes to the
log by default (`CRONOS_AUDIT=off` to stop it), because every deployment
already collects, indexes and retains stdout — and a commercial sink registered
at build time wins over it. Each entry carries the id of the request that
caused it, so the audit and the request log are one story.

The schema is versioned and forward-only. Each migration runs once, inside a
transaction, and records itself in the same transaction — so a failure halfway
leaves the database where it started, and a database migrated by a newer cronos
is refused rather than written to by an older one.

With both configured, the store is the truth once it holds anything, and the
directory is the bootstrap: an empty store adopts it whole, and from then on
what runs is what the store holds. That is what makes editing in the portal
safe — an edit is not reverted by the next deploy, and a definition deleted
through the API does not come back because its file is still on disk. The
startup log says which happened, so a file you changed and a server that
ignored it is a line you can read rather than a mystery.

The SQL store runs against both, and is tested against both. SQLite for the
logic — tenancy, versioning, history — and a real Postgres in CI for what
SQLite cannot show: `BLOB` is not a type Postgres has, a boolean column
declared `INTEGER` does not scan into a Go `bool`, and neither of those is
visible until a Postgres runs the same code.

```bash
CRONOS_POSTGRES_DSN=postgres://…  go test ./internal/adapter/store/sql/
```

Without the variable those tests skip, loudly. A skip that reads like a pass is
how the gap survived.

## Datasources

A dataset names the sources it reads, and that is where its rows come from:

```yaml
spec:
  sources:
    - ref: warehouse
```

Each datasource is opened once at startup with its own pool bound, its own
statement timeout and its own row cap — somebody else operates that database
and decided what a reasonable answer from it looks like. A source that will not
open stops the server rather than being skipped: three of four warehouses
reachable means three-quarters of the reports work and the rest fail at six in
the morning.

The pool defaults to sixteen connections, kept idle rather than churned, and
retired after thirty minutes. All four are overridable per source, and all four
have defaults because the alternative was database/sql's — the first of which
is unlimited. Bounding it made cronos faster, which is the ordinary result:
measured against Postgres, throughput at sixty-four concurrent renders went
from 889/s to 2,008/s and p50 from 62ms to 31ms, because unbounded concurrency
against a database is congestion rather than parallelism. `make load` is the
harness those numbers came from.

The row cap **refuses** rather than truncates. A report that quietly stopped at
a million rows is a wrong answer presented as a right one, and the reader has
no way to tell.

A dataset naming two sources is a join across databases and needs
`-tags duckdb`. Without it the error says so, rather than failing to find a
driver.

With no datasources defined, every dataset reads `CRONOS_DSN`. That is the
development path, and it stays because a demo needing four YAML files before it
shows a number is a demo nobody runs.

## The portal, on real data

With no `VITE_CRONOS_API` the portal runs on **sample data** — which is what
makes the interface workable before a server exists, and what every browser
suite exercises. Connected, it shows real numbers.

```bash
TOKEN=$(go run ./cmd/cronos-token -audience portal -role editor \
  -org acme -project finance -subject dewi)

cd apps/portal && VITE_CRONOS_API=http://localhost:8787 VITE_CRONOS_TOKEN=$TOKEN bun run dev
```

The mode is announced in the shell rather than inferred. A reporting tool
showing invented figures that look real is the worst thing this product could
do, so "these are samples" is a statement the interface makes.

### Signing in

```bash
export CRONOS_STORE_DSN=postgres://…        # users live in the definition store
echo "correct horse battery staple" | go run ./cmd/cronos-user \
  -email dewi@acme.example -name Dewi -org acme -project finance -role editor
```

A command rather than a first-run web form: a deployment that shows "create the
first admin" to whoever reaches it is a deployment where the first visitor is
whoever found the port. The password is read from a terminal or piped, never
from a flag — a password on a command line is in the shell history, in the
process list, and in whatever shipped that history somewhere.

Sign-in is rate limited twice: by address, which stops one machine working
through a password list, and by account, which stops a thousand machines each
trying twice against one known address. A rate rather than a lockout — a
lockout is a denial of service anybody can trigger against a real person by
guessing wrong on their behalf. Being throttled reads exactly like a wrong
password, because a different answer would tell somebody the account exists.
Opening a share is limited too: the id is the credential and there is no other,
so that rate is what stands between the id space and somebody enumerating it.
Set `CRONOS_BEHIND_PROXY=1` only where something in front sets
`X-Forwarded-For` — believing it without a proxy keys every limit by a value
the caller chooses, which is not a limit.

A session lasts eight hours and does not refresh; a token that renews itself is
a permanent credential wearing an expiry. Every sign-in failure reads the same,
and an unknown address costs the same to refuse as a known one — otherwise a
stopwatch tells you who has an account.

The portal never holds the admin key. That is a shared secret a deployment
pipeline has, and a browser is the one place it must not be — anything in a
browser is in a devtools console, a screenshot and a support ticket. It uses a
**portal token**, and the server refuses to accept one for the embed endpoints
or an embed token for the portal's. Real sign-in is not built yet; the
mechanism it will issue against is, and it is enforced now so nothing is open
in the meantime.

## Run history

Set `CRONOS_STORE_DSN` and every burst is recorded: which definition version
ran, what period it covered, and where each document went.

```bash
curl -H "Authorization: Bearer $CRONOS_ADMIN_KEY" localhost:8787/v1/runs/run_1786458499_322e1a57
#   c-1 | file | c-1 | delivered | attempts 1 | 25063 bytes
```

That is the auditor's question — what did this customer receive, at which
address, after how many attempts — and it is why the version is recorded
alongside: a run naming one can be replayed against exactly the document that
produced it.

A run is written when it *starts*. A burst that crashed halfway is precisely
the one somebody needs to look at, and a history that only contains finished
runs is a log of successes. Recording never fails a delivery: a document that
reached a customer has reached them whatever the audit table thinks.

The endpoint is behind the admin key and never the embed token — a run record
names every recipient of a burst.

## Schedules

`CRONOS_SCHEDULER=1` arms them. Off by default, because two instances both
running the same bursts deliver every customer two documents — which one
schedules is a deployment decision, not a default.

```bash
CRONOS_SCHEDULER=1 CRONOS_DELIVERIES=/tmp/out ./bin/cronosd
# armed schedule=monthly-statements next=2026-09-01T06:00:00+02:00
# schedule delivered schedule=monthly-statements period="August 2026" recipients=3
```

It does not catch up. A server down for a week comes back and runs each
schedule once, at its next due time — nobody wants seven copies of last week's
invoices. It does not overlap either: a run still going when the next is due is
skipped and logged, because two bursts of the same statements racing each other
deliver every customer two documents that disagree.

The period comes from the cadence rather than being declared. The span between
the previous firing and this one *is* the period, whatever the cron expression
happens to be, which is what `{{ .run.periodStart }}` and `{{ .run.periodEnd }}`
resolve to.

`onFailure.retries` retries transient failures with exponential backoff.
Permanent ones are not retried: a relay that is down and an address that does
not exist both fail, and retrying the second is three more rejections and a
burst that takes an hour longer to reach the same answer.

Delivery channels register only when configured — `CRONOS_SMTP_HOST` and
`CRONOS_SMTP_FROM` for email, `CRONOS_S3_ACCESS_KEY` and `CRONOS_S3_SECRET_KEY`
for object storage. A channel nobody set up is absent rather than present and
always failing.

## What is here

| | |
| :--- | :--- |
| `internal/core/` | Definitions, validation, query compilation. No external dependencies. |
| `internal/app/run/` | Report → SQL → rows → what a viewer draws |
| `internal/adapter/` | YAML, `database/sql`, the Typst renderer, the HTTP API |
| `internal/adapter/driver/duckdb/` | Federation — one query over a warehouse, a lake and a spreadsheet. cgo, so `-tags duckdb` |
| `apps/portal/` | The authoring UI — React 19, Mantine, Tailwind, PWA |
| `packages/embed/` | `<cronos-report>`, 3.2 KB gzipped, framework-agnostic |
| `packages/react/` | A React wrapper for it |
| `ee/` | Commercially licensed. Nothing under `internal/core` may import it. |

## Checks

```bash
make check          # build, vet, gofmt, test, licence boundary, typecheck, budgets
make ui             # every browser suite, plus the embed against a real server
make live           # just the embed against a real cronosd
make pdf            # render a sample statement and open it
make duckdb         # build and test federation (cgo, several hundred MB)
make xlsx-oracle    # install openpyxl, so the spreadsheet tests run rather than skip
```

Federation is behind a build tag on purpose. DuckDB is a C++ library, so it
needs cgo and adds several hundred megabytes to a module download — neither
cost belongs in a build that only ever reads one Postgres. Without the tag
cronos is pure Go, cross-compiles to anything, and asking for federation is a
clear error rather than a missing symbol.

All of it runs on every push — `.github/workflows/check.yml`, split so a
bundle over budget and a broken query compiler are different conversations. The
Go job runs with `-race`.

`make live` is the one that matters most. The Go tests prove the server
computes the right numbers and the package checks prove the component renders
what it is handed; only this proves they agree about the shape in between.

## Reading order

[docs/product.md](docs/product.md) — what this is for, and for whom ·
[docs/report-format.md](docs/report-format.md) — the file format ·
[docs/tenancy.md](docs/tenancy.md) — the three levels of scope, and how they
fail · [docs/rendering.md](docs/rendering.md) — paginated PDF ·
[AGENTS.md](AGENTS.md) — the rules the code is held to.
