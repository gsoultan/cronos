# cronos

A report engine and report repository: define reports over SQL databases, big data
sources and CSV files, render them as tables, charts and paginated documents, and
schedule and distribute them — embeddable in other applications.

## Local development

```bash
make setup     # verify toolchain, install dependencies, smoke-test the build
make dev       # run the API and portal together
```

`make dev` starts both with prefixed output and stops both on one Ctrl-C. If one
side goes down the other keeps running — a crashed API should not end a session
you were using to work on the portal.

| | |
| :--- | :--- |
| `make dev-web` / `make dev-api` | run one side only |
| `CRONOS_WEB_PORT` · `CRONOS_API_PORT` | override ports (defaults 5173 / 8080) |
| `make check` | everything CI runs — build, vet, license boundary, typecheck, lint, bundle budgets |
| `make shots` | drive the portal in headless Chrome, write screenshots |
| `make help` | all targets |

**The API is a stub.** `cmd/cronosd` has no HTTP server yet, so it prints its
wiring and exits — `make dev` reports that and keeps the portal up. The portal
runs on mock data until the engine exists. See [docs/product.md](docs/product.md)
for what is being built and in what order.

Requires Go 1.26+ and [Bun](https://bun.sh) 1.3+.

## Licensing

cronos is source-available under the [Business Source License 1.1](LICENSE), which
converts to the Apache License 2.0 on **2030-08-10**.

**You may use cronos freely, including in production, for internal use within your
own organization.** No fees, no user limits.

**A commercial license is required** to offer cronos to third parties as a hosted,
managed, or embedded reporting or analytics service — for example, embedding its
reports or UI into a product you sell, or running it as a service for your
customers. See the Additional Use Grant in [LICENSE](LICENSE) for the exact terms.

For commercial licensing, contact the maintainer.
