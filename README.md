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

The SQL store is tested against SQLite — its statements use nothing outside
`ON CONFLICT` and ordinary predicates, so the tenancy and versioning logic
under test is the logic Postgres runs. Postgres-specific behaviour around types
and concurrency is **not** covered yet; that wants a container this repository
does not have.

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
```

Federation is behind a build tag on purpose. DuckDB is a C++ library, so it
needs cgo and adds several hundred megabytes to a module download — neither
cost belongs in a build that only ever reads one Postgres. Without the tag
cronos is pure Go, cross-compiles to anything, and asking for federation is a
clear error rather than a missing symbol.

`make live` is the one that matters most. The Go tests prove the server
computes the right numbers and the package checks prove the component renders
what it is handed; only this proves they agree about the shape in between.

## Reading order

[docs/product.md](docs/product.md) — what this is for, and for whom ·
[docs/report-format.md](docs/report-format.md) — the file format ·
[docs/tenancy.md](docs/tenancy.md) — the three levels of scope, and how they
fail · [docs/rendering.md](docs/rendering.md) — paginated PDF ·
[AGENTS.md](AGENTS.md) — the rules the code is held to.
