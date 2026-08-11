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

## What is here

| | |
| :--- | :--- |
| `internal/core/` | Definitions, validation, query compilation. No external dependencies. |
| `internal/app/run/` | Report → SQL → rows → what a viewer draws |
| `internal/adapter/` | YAML, `database/sql`, the Typst renderer, the HTTP API |
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
```

`make live` is the one that matters most. The Go tests prove the server
computes the right numbers and the package checks prove the component renders
what it is handed; only this proves they agree about the shape in between.

## Reading order

[docs/product.md](docs/product.md) — what this is for, and for whom ·
[docs/report-format.md](docs/report-format.md) — the file format ·
[docs/tenancy.md](docs/tenancy.md) — the three levels of scope, and how they
fail · [docs/rendering.md](docs/rendering.md) — paginated PDF ·
[AGENTS.md](AGENTS.md) — the rules the code is held to.
