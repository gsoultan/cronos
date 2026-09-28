# cronos — engineering guidelines

Binding rules for this repository. A change that violates one is not done, however
well it works.

## Locked decisions

| Decision | Choice | Because |
| :--- | :--- | :--- |
| Paginated renderer | **Typst** (proven — `docs/rendering.md`) | Real typesetting: page breaks, grouping and subtotals are semantics, not CSS hints. Low memory per render, so a 5,000-customer burst does not need a browser farm. Cost accepted: a non-Go dependency and a template syntax authors must learn. |
| Query / federation engine | **DuckDB** (built, `-tags duckdb`) | One engine covers SQL databases, Parquet/CSV on object storage, and cross-source joins — the three data-source requirements without a second concept. |
| Result transport | **Arrow record batches** | Columnar and zero-copy from driver to renderer; the data-plane contract that makes million-row exports survivable. |
| License boundary | **Import graph** | Enforced by `scripts/check-license-boundary.sh` against the real build graph, not by convention. It holds in distribution too: `make dist` writes the community binaries and `cronosd-ee` as separate archives, each carrying the LICENSE that covers it. |
| Distribution | **Archives and an image** | A container is one way to run cronos, not the only one. Anything that ships has to be in both, or a tool exists in one channel and the documentation is wrong for half its readers. |

## Developer Profile Panel

Non-trivial changes are worked as a pair: adopt the **Driver** profile that owns the
code you touch, then re-read your own diff as the **Challenger** whose budget the
change most likely breaks. Name both in the task summary (`Driver: perf · Challenger: sec`).

| Profile | Owns | Vetoes | Proof required |
| :--- | :--- | :--- | :--- |
| **arch** | Layering, package boundaries, interface shape | A God interface; an interface declared in the producer package; core importing `ee/` | `scripts/check-license-boundary.sh` green; no package cycle |
| **perf** | Data plane: query compilation, streaming, render, burst concurrency | Materialising a full result set in memory; per-row allocation on the data plane; unbounded fan-out | Benchmark at 1M rows, allocations/row reported |
| **sec** | Principal resolution, RLS, parameter binding, secret handling | String-interpolated SQL; an RLS bypass flag; a fail-open default; a cache key missing tenant | Test proving a cross-tenant read returns zero rows |
| **fe** | Bundle budgets, render performance, embed payload | Mantine or the builder reaching the embed bundle; an unvirtualised table; a blocking main-thread transform | CI bundle report under budget |

Standing truths: a fast path that skips a check is a vulnerability; a check that
allocates per request is a regression; a cache that ignores who asked is a data leak;
any map keyed by attacker-supplied input needs a bound and an eviction; every bug fix
ships a test that fails before and passes after, with the root cause named in one sentence.

---

## Backend (Go)

### Two planes, two rule sets

The single most important structural rule. Clean architecture protects what changes;
the data plane gets a narrow stable contract instead, because layer hops and DTO
mapping per row do not survive a million-row export.

| | Control plane | Data plane |
| :--- | :--- | :--- |
| Scope | Definitions, repository, versioning, scheduling, ACL, delivery config | compile → execute → stream → render |
| Volume | Low | Millions of rows |
| Architecture | Full clean architecture: entities, use cases, ports, adapters, mapping at boundaries | Streaming pipeline, columnar batches, one contract end to end |
| Mapping | At every boundary | **None per row.** No entity mapping, no layer hops |
| Guideline application | All rules as written | All rules except per-boundary mapping |

### Composition and interfaces

Go has no inheritance. "OOP" here means **composition + programming by interface**;
embedding is for composition, never for an is-a hierarchy.

- **Interfaces are declared by the consumer**, not beside the implementation. There is
  no central `interfaces/` package — in Go that is a God package and an import-cycle
  factory. Accept interfaces, return structs.
- One interface per file, one exported struct per file. Unexported helper types may
  live in their owner's file.
- Max 15 methods per interface — that is a ceiling, not a target. Aim for 3–7.
  Compose smaller interfaces rather than growing one.
- Max 50 lines per function.

### Layout

```
cmd/cronosd            BSL binary. Must never reach ee/.
cmd/cronosd-ee         EE binary. Blank-imports ee/.
internal/
  core/                Domain entities, value objects, domain errors. Zero external deps.
    definition/          Report · Dataset · Schedule            ✓
    document/            What a paginated document *is*         ✓
    history/             What a run actually delivered          ✓
    query/               Compilation: binding, row scope, filters, blocks  ✓
    principal/           Identity, tenancy                      ✓
    access/              Who may open which report              ✓
    identity/            Users and passwords (SSO is ee/)       ✓
  app/                 Use cases. Declares the ports it needs, as consumer-side interfaces.
    run/                 Report → SQL → rows → view             ✓
    publish/             Validate, version, store                ✓
    burst/               One document per recipient, bounded     ✓
    schedule/            Cron loop, no catch-up, no overlap        ✓
    vault/               A project's secrets, sealed, write-only   ✓
  adapter/             Port implementations.
    codec/yaml/          The file format authors write, read and written ✓
    codec/jrxml/         JasperReports in. The 80%, and what it refused ✓
    api/                 Embed + portal + management, CORS      ✓
    store/file/          Definitions from a directory           ✓
    store/sql/           Multi-tenant. Postgres + SQLite, both tested ✓
    driver/registry/     One connection per datasource          ✓
    driver/sql/          Anything database/sql speaks           ✓
    driver/duckdb/       Federation. cgo, `-tags duckdb`        ✓
    render/paginated/    Typst PDF                              ✓
    basemap/             Map tiles: OpenStreetMap, Mapbox, Google ✓
    render/spreadsheet/  XLSX, written by hand, read by openpyxl ✓
    deliver/file/        Documents to a directory                ✓
    deliver/email/       SMTP with a MIME attachment             ✓
    deliver/s3/          Object storage, S3 API                  ✓
    deliver/             sftp · webhook
  platform/
    token/               Embed tokens — not JWT, see its doc.go ✓
    config/              Environment                            ✓
    h3/                  H3 cells, ported from Uber's C (Apache 2.0) ✓
  extension/           License seams. See ee/doc.go.            ✓

✓ = built and under test. The rest is named so the shape is agreed before
something lands in the wrong package, not because it exists.
ee/                    Commercially licensed. Own LICENSE. May import core; core may not import it.
```

### Patterns, and the problem each solves

| Pattern | Applied to | Solves |
| :--- | :--- | :--- |
| Registry | Drivers, renderers, delivery channels | Extension without modifying the core |
| Strategy | `renderer:` per output profile | One definition, three output classes |
| Decorator | RLS + audit wrapping the executor | Makes unconditional RLS structural — no path can skip it |
| Builder | dataset + params + principal → query plan | The one place parameter binding is enforced |
| Repository | Definition versions | Content-addressed history, reproducible runs |
| Pipeline | compile → execute → stream → render → deliver | The data plane's single contract |

Do not add a pattern that is not solving a problem named here.

### Performance rules

- Results are bounded everywhere, and streamed almost everywhere. A render never
  holds more than a page: a table block pages at the query level, so the
  interactive path answers in two milliseconds and 25MB whether the table has
  fifty rows or two million. A delivery holds one recipient's rows. The exception
  is a spreadsheet export, which materialises the whole set in `[][]any` because
  a workbook is written as a whole — 32MB measured for the 100,000 rows
  `run.ExportLimit` allows, times six columns. That is the number to check
  against if the cap ever rises; this entry used to say nothing materialises a
  full result set, which was true of two paths out of three.
- A map with more places than a payload holds is gathered in the database, not
  read into Go: cells a few pixels wide (`query.BuildMapCells`), hexagons and
  regions over every row, a view at a time as the reader zooms. 0.13 Go
  allocations per source row at a million — `BenchmarkLargeMapOpens` — because
  the rows never cross the wire; what does is bounded by `query.MaxCells`. The
  one exception is an `h3` layer, because H3 has no SQL form in most
  warehouses: it streams at most `run.h3Most` (200,000) places or indexed cells
  through Go, keeping the cells and never the rows, and says so past that.
- Columnar batches (Arrow) on the data plane, not `[]map[string]any`.
- Every datasource carries a statement timeout and a row cap. No unbounded query.
- Burst fan-out is bounded by `concurrency`, with backpressure to the renderer.
- A server-side cache is keyed by whoever may read what it holds. Three exist:
  whether an account still stands, and its confinement, by the token's subject
  — an account id, one person in one tenant, so narrower than a tenant key
  rather than missing one; resolved
  secrets by organisation, project and name, because a secret is the project's
  and no response carries one (`vault.Cache`, bounded, and
  `TestAnotherProjectReadsNoneOfIt` fails if the tenant leaves the key); and
  Google tile sessions by a digest of the key. The browser's cache carries no
  principal at all; it is emptied when the session changes. This entry once said
  there was one cache, and before that that every key includes the tenant and
  the definition version — neither was true for long; see docs/tenancy.md.

---

## Frontend

### Two artifacts, non-negotiable

PWA and embedding are in tension: ISV customers will not accept a Mantine-sized
payload in their application, and the host is a client-side SPA — React built
with Vite, Vue, and so on — not a server-rendered framework.

**There is no iframe path.** An iframe is third-party HTML in a browsing
context: clickjacking, navigation escapes without a strict `sandbox`, storage
partitioning that breaks auth, and `postMessage` to get wrong. Embedding is a
custom element in a shadow root, with `@cronos/react` for the one framework
that needs a wrapper.

| | `apps/portal` | `packages/embed` (+ `packages/react`) |
| :--- | :--- | :--- |
| Audience | Report authors, internal users | Our customers' end users |
| Stack | React 19.2+, Mantine 9, TanStack Router/Query/Form | Web component, framework-agnostic |
| PWA / service worker | Yes | No |
| Builder UI | Yes | **Never** |
| Budget | See below | ≲40 KB gzip (**34.3 KB** today, gated by `bun run size`; the interactive map took it from 14.8, painting a large one's cells from 20.8, maps that filter and draw overlays from 24.4, shading, glyphs and size keys from 26.5, labels, radii, flow heads and themed basemaps from 27.8, and maps that play through time from 30.1. Charts redrawn at their real size took it to 35.8, and stripping the stylesheet's comments at build — `packages/charts/build/sheet.ts`, 11 KB of prose inside a string no minifier touches — brought it back; column, radar and bullet charts took it to 33.6, and histograms and box plots to 34.3) |

### Toolchain

- Bun · Vite 8 (Rolldown) · TypeScript 7 · React 19.2+ · Mantine 9 · Tailwind CSS 4
- TanStack Query, Form, Router
- **oxlint, not typescript-eslint.** TypeScript 7.0 ships without a stable programmatic
  API until 7.1, so typescript-eslint cannot run on it. Mantine v9.5 made this same move.

### Styling

Tailwind v4, configured in CSS. Three rules:

- **Design tokens are Tailwind theme variables.** `@theme` in `src/theme/index.css`
  names them into Tailwind's namespaces, so `--color-surface` *is* `bg-surface`.
  A component that writes a raw hex or an arbitrary pixel value has left the system.
- **No `dark:` variants.** Dark mode redefines the same theme variables under
  `[data-theme='dark']`, so every `bg-surface` switches on its own. Components
  never spell the theme twice. Adding a colour means adding both steps.
- **Cascade layer order is `theme, base, cronos-charts, mantine, components,
  utilities`.** Mantine is imported via its `.layer.css` variants so a utility can
  override a component style without `!important`. The unlayered files sit outside
  the layers entirely and beat every utility — that is the failure mode to watch
  for, and **a stylesheet does not have to be a file to hit it.**

  `document.adoptedStyleSheets` is unlayered and sorts after every `<link>`, so a
  sheet adopted at runtime beats the whole cascade. `@cronos/charts` is written for
  a shadow root, where the boundary does the scoping and the selectors can be bare
  — `.grid`, `.panel`, `.bars`, `*`. Adopted into the portal's document those are
  global *and* unlayered, and the shell is a `<div class="grid grid-cols-[248px_…]">`:
  from the first chart a session drew until it ended, the navigation column was half
  the window. The same rule flattened the report's block grid, and the sheet's
  literal `--cr-surface: #fff` beat the themed one, so charts stayed white on a dark
  page. Every screen without a chart on it looked correct, which is why it lived so
  long.

  Adopting a sheet into the document therefore means both wrappers, which is what
  `documentCss()` returns: `@scope` so an ordinary class name cannot reach the page
  around it, and `@layer` so the host still owns the theme. The layer name has to be
  ordered in the statement above or it is appended last and wins — and ordered
  **above `base`**, because below it is below preflight (`*{margin:0;padding:0;border:0}`),
  which leaves a panel with its background and no edges. `ServerChart.test.ts` pins
  both ends of that.

Logic with rules worth stating — SQL generation, filter compilation — carries unit
tests (`bun test`). Anything that can be asserted without a browser should be.

Tests and screenshot scripts select on `data-testid` or accessible roles, never on
styling classes. A class is a styling decision and will change; a test that depends
on one breaks on every restyle.

### Bundle budgets

Enforced in CI on every PR; a build over budget fails.

| Artifact | Raw | Gzip |
| :--- | :--- | :--- |
| Portal initial route | ≤ 500 KB | ≤ 150 KB |
| Any lazy chunk | ≤ 500 KB | — |
| Embed bundle | — | ≤ 40 KB |

Lazy-loaded, never in an initial chunk: charting library, PDF viewer, report builder,
spreadsheet export.

The same rule applies to CSS, and it is the one that drifts, because a stylesheet
imported once covers every route for ever. `theme/mantine.css` is what the first
screen renders — TextInput, PasswordInput, Button — and `theme/mantine-deferred.css`
is everything else, fetched by Shell on mount. Adding a Mantine component means
choosing between them, and the question is: does the sign-in form render it?

### Performance rules

- Every table is virtualised. A report can return a million rows; the DOM cannot.
- Row transforms, aggregation and chart data prep run in a **web worker**. The main
  thread is for painting.
- Service worker precaches the application shell, and no API response. Offline
  viewing of results is not built: a cache keyed by URL serves one principal's
  data to the next person on that browser, so it needs a cache named per
  principal and emptied on sign-out. The rule that claimed to do this matched
  three routes that do not exist and had never cached anything.
- The TanStack Query cache is emptied when the session changes, in one place
  (`lib/queryClient.ts`). A cache that ignores who asked is a data leak, and no key
  here names who asked — `['catalog']` is the same key for everybody. This entry
  used to claim the keys carried the tenant; they never did, and a browser driven
  through sign-out and sign-in as another organisation was served the first
  organisation's catalogue under the second one's name. Naming the tenant in each
  key is the fix that only holds until a hook forgets; the cache is session state,
  so it ends with the session. Held by `scripts/live-handover.sh`.

---

## Definition of done

1. `go build ./... && go vet ./...` clean
2. `scripts/check-license-boundary.sh` green
3. Frontend bundle report under budget
4. Driver and Challenger profiles named in the summary, Challenger vetoes answered
5. Bug fixes ship a test that fails before and passes after
