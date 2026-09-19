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

`driver` is one of `postgres`, `mysql`, `sqlserver`, `sqlite`, `duckdb` or
`object-store`. `mssql` is accepted as another name for `sqlserver`.

SQL Server takes an ordinary connection string:

```yaml
spec:
  driver: sqlserver
  dsn: ${secret:erp_dsn}   # sqlserver://reader:…@sql.acme.internal:1433?database=erp
```

Two things about it differ from the others, and both are deliberate:

- **Weekly grouping is refused.** `DATEDIFF(week, …)` counts Sunday boundaries
  whatever `SET DATEFIRST` says, so the same report grouped by week would give
  one answer here and another on Postgres, where a week begins on Monday. A
  chart that is quietly a day out is not read as an error by anybody, so the
  engine says so instead. Day, month, quarter and year all work.
- **Dates are truncated with `DATEADD`/`DATEDIFF` rather than `DATETRUNC`.**
  `DATETRUNC` is SQL Server 2022 and later, and a great many of the servers a
  reporting tool meets are 2016 and 2019 — they are what sits behind an ERP.

Object storage and files use the same kind, which is how one report reaches SQL,
big data and CSV without a second concept:

```yaml
apiVersion: cronos.dev/v1
kind: DataSource
metadata:
  name: events-lake
spec:
  driver: object-store
  uri: s3://acme-lake/events/    # a prefix, never a single file
  format: parquet                # parquet | csv | json
  region: eu-central-1
  credentials: ${secret:lake_creds}
  # endpoint: http://minio.internal:9000   # only when the store is not the cloud's own
```

**`uri` is a prefix, not a file.** Everything under it matching the format is
read as one table, recursively, so a lake partitioned by date needs one
datasource rather than one per day. Pointing it at `events/2026-01.parquet` is
refused at startup with the pattern it looked for — put the file in a directory
and name the directory.

Partitions are unioned by name, so a column added in March and a decimal
widened in June both keep reading. A column that means two different things in
two partitions is still two different things; this reconciles schemas, not
mistakes.

**`credentials` holds `key=value` pairs**, and belongs in a secret rather than
in the file:

| Field | For |
| :--- | :--- |
| `key_id`, `secret` | S3, GCS and R2. Together, always — either alone is refused. |
| `session_token` | Temporary credentials. |
| `account_id` | Cloudflare R2, which addresses a bucket by account. |
| `account_name`, `account_key` | Azure Blob. Together, and named as the Azure portal names them. |
| `provider` | `chain`, where the credential comes from the environment rather than the file. |

```
key_id=AKIAEXAMPLE;secret=wJalrXUtnFEMI/K7MDENG
account_name=acmelake;account_key=0Vn2Iq…==
```

Set `credentials: chain` instead to take them from the environment — an
instance profile, a projected service-account token, a local AWS profile.
Nothing is read from the definition then, which is the shape to prefer where
the deployment already has an identity. Azure needs the account named even
then, because a chain answers who the caller is and not which account they are
reaching: `provider=chain;account_name=acmelake`.

A credential from the wrong cloud is refused by name rather than passed
through — `account_name` on an `s3://` bucket builds a statement with a
parameter the store has never heard of, and the message for that is better
written here than quoted from a database.

**Azure Blob is `az://`, `azure://` or `abfss://`**, all three, because three
tools emit three of them. Azure takes no `region` — the account resolves to
one — and its `endpoint` goes inside the connection string cronos assembles
rather than beside it, which is what makes an emulator or a private cloud
reachable.

Omit `credentials` entirely for a public bucket. A `region` on its own is fine
and still creates the secret that carries it.

Credentials are scoped to the `uri` they were given with, so two lakes with a
key each authenticate as themselves rather than as whichever loaded last. They
reach `s3://`, `gs://`, `r2://` and Azure's three spellings; on any other
scheme a credential is refused rather than ignored, because one that does
nothing is one nobody rotates.

**`endpoint` is for a store that is not the cloud's own** — MinIO, Ceph, an
appliance on an internal address. Give it a scheme: it decides TLS, and a
plaintext store should be one somebody asked for rather than one they were
given. Buckets are then addressed by path, because bucket-as-subdomain needs a
DNS entry per bucket and an internal address does not have one.

Leave it out for AWS, GCS or R2. Setting it wrongly is worth recognising: the
request goes somewhere that is not your store, and an object listing that finds
nothing comes back as an empty lake rather than as a store that was never
reached.

`scripts/live-objectstore.sh` exercises this against a real server — that a key
authenticates, that a wrong one stops the read, that a refusal does not carry
the secret into a log, and that two lakes with a key each stay separate.

## Dataset

A dataset reading more than one source, or reading an object store, is compiled
against a query engine that mounts them — which needs a build made with
`-tags duckdb`. Without one the error says so and names the sources it could not
join, rather than failing to find a table.

Each source is mounted under the name the query uses for it. A datasource name
is a slug and may hold dashes; a mounted name becomes an identifier in SQL and
may not, so `as:` is required wherever the two differ. A dataset reading one
ordinary database needs none of this and is compiled straight against it.

```yaml
apiVersion: cronos.dev/v1
kind: Dataset
metadata:
  name: invoices
spec:
  sources:
    - ref: warehouse
    - ref: events-lake
      as: events        # `events-lake` is a datasource name, not a SQL one

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

That promise is kept by compilation, not by the interface remembering: building
a block's query returns a coverage alongside it, naming the filters that reached
this dataset and the filters that did not. Nothing downstream can reconstruct
which happened, so it is reported rather than inferred.

A declared filter that is currently blank still *covers* a block — it is simply
not narrowing anything yet. Only a missing binding makes a block unaffected, and
only that is what the interface should announce.

Values are compared through a fixed set of operators — `eq` `ne` `lt` `lte` `gt`
`gte` `in` `between` `contains` `isNull` `notNull` — because the operator arrives
with the request. A caller chooses among comparisons; a caller never supplies
one. The bound field comes from `bind:` and is the only part of a filter
predicate that reaches SQL as text rather than as an argument, so it is checked
against the dataset's published fields on save and again at compile.

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

### The filter bar

`spec.filters` declares the controls above a report. Each says what it narrows
in every dataset the report reads, because a report's blocks may read different
ones and guessing is how a filter silently applies to half a screen:

```yaml
spec:
  filters:
    - name: period
      label: Period
      type: date
      bind: {invoices: issued_at, shipments: dispatched_at}
    - name: status
      label: Status
      type: enum
      values: [sent, overdue, paid]
      bind: {invoices: status}
```

`control:` picks the interface each filter is operated through. A status with
four values and a status with forty want different ones, and only the author
knows which:

| type | controls | default |
| :--- | :--- | :--- |
| `date` | `range`, `calendar`, `presets` | `range` |
| `number` | `range`, `slider`, `search` | `range` |
| `enum` | `dropdown`, `radio`, `checkboxes` | `dropdown` |
| `bool` | `dropdown`, `radio` | `dropdown` |
| `string` | `search` | `search` |

A control that cannot operate its type is refused when the report is stored — a
calendar on an enum is nonsense, and a renderer handed one would have to choose
between drawing something wrong and ignoring what the author asked for. The
default is resolved on the server, so the portal and the embed draw the same
filter rather than each applying a default of its own.

A dataset with no entry in `bind` is **unaffected**, which is a legitimate
outcome rather than a mistake — a Period filter has nothing to say to a dataset
of current stock levels — and every block reports it, so it is stated rather
than discovered.

The embed viewer draws the bar itself: a date filter renders a from/to pair and
sends `between`, `gte` or `lte` depending on which ends were filled, an enum
renders a list and sends `in`, and free text sends `contains`. A host page that
already has its own controls sets `controls="none"` on the element and keeps
driving the `filters` property. The bar used to be described here and drawn
nowhere, so every host built one and reimplemented which operator a date range
sends.

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
shared vocabulary; a renderer ignores properties it cannot honour (`pageBreak`
means nothing interactively).

Charts are drawn by both. The paginated renderer typesets them as vector marks,
so a PDF carries the same chart the browser does — the same palette, the same
tick labels, and the same numbers, because the arrangement is worked out once on
the server and both renderers place what they are given. A layout of charts and
no table is a legitimate paginated output. The one exception is `map`, which
prints a line saying to open the report in a browser: the geometry reaches the
viewer as SVG paths, and a typesetter wants vertices.

This paragraph used to say a bar chart in a PDF rendered as a static image, which
no code ever did — charts were dropped from a paginated output entirely, with
nothing saying so.

### Charts

`kind: chart` with a `chart:` type, rather than a block kind per visualisation —
so adding a chart type costs every renderer one case rather than a new concept.
The type is a closed set, checked when the report is stored:

| `chart` | Reads | Draws |
| :--- | :--- | :--- |
| `bar` | `x` dimension, `y` measure | Horizontal bars |
| `line` | `x` dimension, `y` measure | A line through every bucket |
| `area` | `x` dimension, `y` measure | A filled line |
| `pie` · `donut` | `x` dimension, `y` measure | Parts of a whole |
| `scatter` | `x` dimension, `xValue` + `y` measures | One dot per category |
| `bubble` | as scatter, plus `size` | Dots sized by a third measure |
| `map` | `y` measure, plus `map:` | Regions, points, density and flows |
| `combo` | `x` dimension, `metrics` | Bars and lines together |
| `funnel` | `metrics`, or `x` + `y` | Stages and the fall between them |
| `waterfall` | `x` dimension, `y` measure | Floating bars and a closing total |
| `heatmap` | `x` + `series` dimensions, `y` | Two dimensions against a measure |
| `gauge` | `y` measure, plus `target:` | One number against something |
| `treemap` | `x` dimension, `y` measure | Area within area |

`series:` names a second dimension to split by, and `stacked: true` stacks the
result — on `bar` and `area` only, because a stacked line has a top edge that
reads as a total nobody measured. Splitting a `pie` is refused for the same
class of reason: it is already part-to-whole.

A scatter's horizontal axis is a measure, so it is `xValue` and not `x`. `x`
stays the dimension that says what each dot *is* — which is also what bounds
the chart, since one dot per row is one dot per however many rows the dataset
has.

```yaml
- kind: chart
  chart: bar
  title: Billed by month and carrier
  x: {field: issued_at, grain: month}
  series: {field: carrier}
  stacked: true
  y: {field: total, aggregate: sum}
```

### Several measures at once

`metrics:` replaces `y` on the types that read a list. A combo draws each
measure its own way against shared buckets; a funnel's metrics *are* its stages,
in the order they are listed, so it has no `x` at all.

```yaml
- kind: chart
  chart: combo
  title: Billed and margin
  x: {field: issued_at, grain: month}
  metrics:
    - {field: total,  aggregate: sum, label: Billed, draw: bar}
    - {field: margin, aggregate: avg, label: Margin, draw: line, axis: secondary}
```

**`axis: secondary` is opt-in, per measure, and never a default.** Two scales on
one plot is the most-flagged mistake in charting: where the two axes line up is
a choice nobody made on purpose, so the chart shows a correlation that is not in
the data. Sharing one scale is the default, and a measure that genuinely needs
its own has to say so — which is the difference between a considered decision
and an accident. The viewer marks such a track "(right)" in the legend, because
a reader otherwise has no way to know that line is not comparable to the bars
beside it.

A funnel comes in two shapes, because data does. Stages as **columns** is how
most warehouses model one:

```yaml
- kind: chart
  chart: funnel
  title: Conversion
  metrics:
    - {field: quoted,  aggregate: count, label: Quoted}
    - {field: ordered, aggregate: count, label: Ordered}
    - {field: paid,    aggregate: count, label: Paid}
```

Stages as **rows** of a dimension works too — `x` and `y`, ordered largest first,
because a funnel that is not descending is a bar chart. Either way the server
computes each stage's share of the first and the fall from the one above it, so
every renderer shows the same percentages.

### Waterfalls, heatmaps, gauges and treemaps

A **waterfall** is `x` and `y` like a bar chart; the difference is drawn, not
queried. The server accumulates the running total and sends where each bar
floats, so the last bar lands where the arithmetic says rather than where a
chain of float additions done twice in two languages happens to put it. A
closing total is appended automatically. Rises and falls take a diverging pair —
blue and red, deliberately not green and red, which is the one pair a colourblind
reader cannot separate on the one chart whose whole point is which side of
nothing a bar is on.

A **heatmap** needs both dimensions: `x` along the top and `series` down the
side. The grid comes back dense, and a pair no row matched is marked `empty`
rather than dropped — "no rows" and "rows totalling nearly nothing" are
different answers, and the ramp's lightest step cannot say both.

A **gauge** folds the whole set to one number and reads it against a `target:`,
which is either a measure or a fixed value:

```yaml
- kind: chart
  chart: gauge
  title: Billed against plan
  y: {field: total, aggregate: sum}
  target: {value: 100000, label: Plan}    # or {field: quota, aggregate: sum}
```

The arc is capped at the target and beating it is said in words beside the
figure, because an arc drawn to 180% wraps past its own start and reads as 80%.

A **treemap** nests when `series` is set: the groups become outer rectangles and
each is laid out again inside its own box. The squarified layout runs on the
server for the same reason the map's projection does — it is an algorithm with a
right answer, and running it in every viewer means three attempts at it. Negative
values are dropped: a treemap encodes quantity as area, and an area cannot be
negative.

Funnel stages and a flat treemap's rectangles take an **ordinal** ramp — one hue,
stepped — rather than eight categorical hues, because swapping two of them
changes what the chart says. A reader should see the order in the colour instead
of discovering there was one by reading the labels.

### Maps

A map is a chart type, and its layers are a list rather than a type per
combination — "shade the regions and put a dot on each depot" is the question
authors ask, and a type per combination is a list nobody can hold.

```yaml
- kind: chart
  chart: map
  title: Parcels by region
  x: {field: region}
  y: {field: parcels, aggregate: sum}
  map:
    layers: [polygon, heat, bubble, flow]
    geometry: region_shape      # a field holding GeoJSON
    lat: depot_lat
    lon: depot_lon
    toLat: destination_lat      # flow layers only
    toLon: destination_lon
    basemap:
      url: https://tile.openstreetmap.org/{z}/{x}/{y}.png
      attribution: © OpenStreetMap contributors
```

| Layer | Needs | Draws |
| :--- | :--- | :--- |
| `polygon` | `geometry` | Regions shaded by `y` — a choropleth |
| `heat` | `lat`, `lon` | Point values spread into a density field |
| `bubble` | `lat`, `lon` | A circle per point, sized by `y` |
| `scatter` | `lat`, `lon` | A dot per point |
| `flow` | `lat`, `lon`, `toLat`, `toLon` | An arc from each origin to its destination |

Geometry is a **field**, not a bundled atlas: a column of GeoJSON, which is what
`ST_AsGeoJSON` returns in PostGIS and in DuckDB's spatial extension. There is no
world map to keep up to date and no list of the countries cronos knows about.

The server projects every geometry to **Web Mercator**, simplifies it, and sends
SVG paths — the same argument that keeps currency formatting on the server.
Shipping rings and a projection to every one of an ISV's end users would cost
more than the whole embed bundle's budget. Web Mercator specifically, because it
is the projection every XYZ tile server already publishes in, so a basemap lines
up with the data for free.

`basemap` is **empty by default and opt-in**. A basemap is a request from the
reader's browser to a third party that cronos would have chosen for them; it
discloses roughly where the data is, and on OpenStreetMap's own servers it is
against the tile usage policy at any volume. The URL must be `https` — an
embedded report is served over https and a browser blocks mixed-content tiles
silently — and `attribution` is required, because every tile source worth using
requires its credit line be displayed.

One block compiles to one query, so a map's layers share a grain. Asking for
polygons *and* points runs at the point grain and adds each region up from the
points under it. That is exact for `sum`, `count`, `min` and `max`, and it is
refused for `avg`: an average of averages is a number nobody measured that looks
entirely plausible. Split the layers across two blocks if you need one.

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

## Sharing

Sharing is an action and a record, not a kind. A one-off send is logged; a link
is a row with an audience and an expiry. Neither is a definition, so neither
gets a YAML file.

The security model is the design. A shared report has to render as *somebody*,
and there are only two honest answers:

| | Renders as | Recipient sees |
| :--- | :--- | :--- |
| **Send** (whatever channels are configured) | You, now | A snapshot of **your** rows, frozen |
| **Link — people in the project** | Them, live | Their own rows, after signing in — possibly fewer |
| **Link — anyone with the link** | You, at creation | A snapshot of **your** rows, frozen, until it expires |

What is deliberately absent is a *live* link that runs as the sender for anyone
holding the URL. That combination looks like a convenience and behaves like an
unauthenticated export of someone else's data, and it is the one shape that
turns row-level security into decoration.

Every option states what the recipient will actually see, next to the control.
"Share" is the word under which data leaves a system.

### Channels

Three channels ship: `email`, `file` and `s3`. Which of them a deployment has
depends on what it configured — `email` needs `CRONOS_SMTP_HOST`, `s3` needs an
endpoint and credentials — and the portal offers the ones it has rather than a
list compiled into the bundle.

A schedule naming a channel the deployment has not got is refused at publish,
with the list of what it does have. It used to be accepted and to fail in the
burst instead, which is the same mistake arriving at 06:00 with nobody watching.

Email rejects attachments over 25 MB.

Telegram is not a channel. A settings panel for it exists in the portal against
a fixture, and earlier versions of this document and of the v0.5.0 changelog
described it as shipping; it does not, and nothing delivers through it.

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
