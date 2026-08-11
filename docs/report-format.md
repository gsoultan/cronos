# Report definition format

Definitions are YAML files. They are the source of truth: the builder UI reads
and writes them, the repository stores versions of them, and git can hold them.
Every file carries `apiVersion` and `kind` so the format can evolve without
breaking stored definitions.

## Four kinds

| Kind | Owns | Typically edited by |
| :--- | :--- | :--- |
| `DataSource` | Connection, credentials, resource limits | Platform / ops |
| `Dataset` | Query, typed fields, parameters, row-level security | Analytics engineer |
| `Report` | Layout and output profiles (interactive, PDF, spreadsheet) | Report author |
| `Schedule` | Cron, bursting, delivery, retry | Ops / business owner |

### There is no separate Dashboard kind

A dashboard is a `Report` whose only output is `interactive`. Every difference
anyone offers between the two turns out to be a property rather than a type: the
number of datasets is a choice, the output medium is the `outputs` list, refresh
is a setting, per-recipient parameterisation belongs to the `Schedule`, and grid
versus page layout is the renderer.

The prior art agrees. Superset has no report artifact — its "Report" is a
schedule. Metabase has none either — a report is a *subscription* on a dashboard.
Sigma collapsed the distinction outright. The counter-example is Power BI, which
ships Report, Dashboard *and* Paginated Report and has spawned an entire genre of
"when to use which" articles; when a distinction needs that much explaining, the
distinction is the problem.

Keeping one kind also makes the product's own claim literally true rather than
true-with-an-asterisk: one definition, several outputs.

## Why `Dataset` is separate from `Report`

This is the load-bearing decision in the format.

Legacy engines embed SQL inside the report (`.jrxml`, `.rdl`). One query per
report means no reuse, security rules copy-pasted per report, and no governed
surface for anything to reason about. Splitting them gives four things at once:

- **Reuse** — many reports bind to one governed dataset.
- **Security in one place** — row-level security is defined on the dataset and
  applied on every path that reads it, including exports and scheduled runs.
- **Typed parameters** — declared once, validated before any SQL is compiled.
- **A grounding surface** — an MCP server or text-to-SQL layer targets datasets,
  not raw tables, so generated queries inherit RLS and field semantics for free.

## DataSource

Credentials are always references, never literals. `${secret:name}` resolves
through the configured secret backend at connect time; a definition containing
an inline password is rejected at validation.

```yaml
apiVersion: cronos.dev/v1
kind: DataSource
metadata:
  name: warehouse
spec:
  driver: postgres
  dsn: ${secret:warehouse_dsn}
  pool:
    maxOpen: 20
    maxIdleTime: 5m
  limits:
    statementTimeout: 30s
    maxRows: 1000000
```

Object storage and files use the same kind, which is how one report reaches SQL,
big data and CSV without a second concept:

```yaml
apiVersion: cronos.dev/v1
kind: DataSource
metadata:
  name: events-lake
spec:
  driver: object-store
  uri: s3://acme-lake/events/
  format: parquet          # parquet | csv | ndjson
  region: eu-central-1
  credentials: ${secret:lake_creds}
```

## Dataset

```yaml
apiVersion: cronos.dev/v1
kind: Dataset
metadata:
  name: invoices
spec:
  sources:
    - ref: warehouse
    - ref: events-lake
      as: events

  query: |
    SELECT i.id, i.customer_id, c.name AS customer_name, i.issued_at,
           i.currency, i.total_cents / 100.0 AS total, i.status
    FROM warehouse.invoices i
    JOIN warehouse.customers c ON c.id = i.customer_id
    WHERE i.issued_at BETWEEN {{ .params.from }} AND {{ .params.to }}

  params:
    - name: from
      type: date
      required: true
    - name: to
      type: date
      required: true
      default: today

  fields:
    - {name: customer_name, type: string,  role: dimension, label: Customer}
    - {name: issued_at,     type: date,    role: dimension, label: Issued}
    - {name: total,         type: decimal, role: measure, aggregate: sum,
       format: currency, currencyField: currency}

  rowLevelSecurity:
    - predicate: customer_id = {{ .scope.customer_id }}
```

### Datasets bind per tile

`spec.dataset` is the report's default. Any block may override it, which is what
lets one report combine invoices and shipments — the thing a separate Dashboard
kind would otherwise have existed to do.

```yaml
spec:
  dataset: invoices          # default for every block

  layout:
    - kind: stat
      value: {field: total, aggregate: sum}
    - kind: bar
      dataset: shipments     # this block reads somewhere else
      x: {field: shipped_at, grain: month}
      y: {field: cost, aggregate: sum}
```

Row-level security follows the dataset, not the report: each block carries the
predicates of whatever dataset it reads. A report combining two datasets applies
both, to their own blocks. Nothing is weakened by mixing.

### Shared filters bind per dataset

A filter bar spanning blocks on different datasets has to say what it means in
each of them. `bind` is that mapping, and it is explicit because guessing is how
a filter silently applies to half a screen.

```yaml
spec:
  filters:
    - name: period
      label: Period
      type: date
      bind:
        invoices: issued_at
        shipments: shipped_at
```

A block whose dataset has no binding for a filter is **unaffected** by it, and
the interface says so on the block rather than leaving it to be discovered. A
filter that quietly applies to some blocks and not others is worse than one that
admits it.

### Definitions belong to a project

A definition's project is its location, not a field in the file — see
[tenancy.md](tenancy.md). Nothing here names an organization or project, so a
definition can be copied between projects unchanged, and renaming a project does
not rewrite every file in it. Names are unique within a project: two projects may
each have a `monthly-statement`, and they are unrelated.

Cross-project references are rejected at validation. A report resolves its
dataset within its own project or not at all.

### Parameters are bound, not interpolated

`{{ .params.x }}`, `{{ .scope.x }}` and `{{ .principal.x }}` compile to
driver-native bind placeholders. The value never enters the SQL string.
Templates that resolve to anything other than a value — a table name, a column,
a fragment — are rejected at validation rather than at run time.

When a parameter genuinely must change query structure (a sortable column, an
optional join), declare it as `type: enum` with an explicit `values` list; each
value maps to a fixed, author-written fragment. There is no path from free text
to SQL structure.

### Row-level security is unconditional

`rowLevelSecurity` predicates are appended to every read of the dataset — the
builder preview, an embedded chart, a CSV export, a scheduled burst. There is no
flag to skip them and no "run as owner" mode. A report that needs to see every
row uses a dataset whose RLS says so.

`{{ .scope.* }}` reads the row constraints carried by the caller's embed token.
This is where an ISV's own customers live: one project, many row scopes. It is
a different mechanism from project isolation, which is structural — a dataset
in another project is *not found*, rather than found and filtered to nothing.
Never model end-customers as organizations.

## Report

One definition, several outputs. This is the point of the format: the same
report serves an embedded interactive view and a paginated PDF, so an
operational report and a dashboard stop being two products.

```yaml
apiVersion: cronos.dev/v1
kind: Report
metadata:
  name: monthly-invoice-statement
  folder: /finance/statements
spec:
  dataset: invoices

  outputs:
    - name: interactive
      renderer: interactive
      layout:
        - kind: stat
          label: Total billed
          value: {field: total, aggregate: sum}
        - kind: chart
          chart: bar
          x: {field: issued_at, grain: month}
          y: {field: total, aggregate: sum}
        - kind: table
          columns: [customer_name, issued_at, status, total]

    - name: pdf
      renderer: paginated
      page: {size: A4, orientation: portrait, margins: 20mm}
      footer: {text: "Page {{ .page }} of {{ .pages }}"}
      layout:
        - kind: table
          columns: [issued_at, status, total]
          groupBy: customer_name
          pageBreak: perGroup
          subtotals: [total]
```

`renderer` selects the backend: `interactive` emits a chart spec for the embed
SDK, `paginated` compiles to PDF via Typst, `spreadsheet` produces XLSX. Header
and footer templates for `paginated` are Typst files (`.typ`), which is why page
breaks, grouping and subtotals are semantics the renderer honours rather than CSS
hints it approximates. Layout blocks are
shared vocabulary; a renderer ignores properties it cannot honour (a bar chart
in a PDF renders as a static image, `pageBreak` means nothing interactively).

## Schedule

Bursting is first-class: one definition fans out to many parameterised runs and
many recipients. This is what operational reporting needs and what dashboard
tools do not have.

```yaml
apiVersion: cronos.dev/v1
kind: Schedule
metadata:
  name: monthly-customer-statements
spec:
  report: monthly-invoice-statement
  output: pdf
  cron: "0 6 1 * *"
  timezone: Europe/Berlin

  burst:
    over:
      dataset: active-customers
    bind:
      customer_id: "{{ .row.id }}"
      from: "{{ .run.periodStart }}"
      to: "{{ .run.periodEnd }}"
    concurrency: 8

  deliver:
    - via: email
      to: "{{ .row.billing_email }}"
      subject: "Your {{ .run.periodLabel }} statement"
      attach: {filename: "statement-{{ .row.id }}.pdf"}

  onFailure:
    retries: 3
    backoff: exponential
    alert: ops@acme.com
```

A burst runs as the principal that owns the schedule, and each row's run still
applies the dataset's RLS. A schedule cannot be used to widen access.

## Storage and versioning

Files are canonical, but the repository is the runtime store. On publish, a
definition is validated, canonicalised and written as a content-addressed
version with an author, a timestamp and a parent. Names resolve to the current
published version; runs record the exact version hash they executed, so a
delivered PDF can always be traced to the definition that produced it.

Git sync is optional and bidirectional: export a folder to files, or point a
deployment at a repository and let publish happen on merge.

## Not in v1

Deliberately excluded, to keep the first release finishable: cross-dataset
joins in a report, alerting/thresholds, a drag-and-drop pixel designer,
write-back, and natural-language query. The format leaves room for each — the
dataset layer is the seam they will attach to.
