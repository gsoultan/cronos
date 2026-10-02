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
`gte` `in` `between` `contains` `isNull` `notNull`, and `within` and `near` for an
area — because the operator arrives with the request. A caller chooses among comparisons; a caller never supplies
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
| `area` | `map` | `map` |

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

An **`area`** filter narrows by place. It binds a pair of fields in each
dataset, the latitude and the longitude, and is set from a map rather than typed
— see "Maps that filter" below:

```yaml
- name: where
  label: Area
  type: area
  bind: {drops: "lat,lon", depots: "lat,lon"}
```

It takes two operators. `within` is a box — south, west, north and east, in
degrees; a west greater than its east is a box across the antimeridian, which
is what a view over the Pacific is. `near` is a distance — a latitude, a
longitude and kilometres — measured on the sphere, exactly: the database keeps
a place 9.99 km away and leaves out one 10.01 km away, over a box that lets it
use an index on the coordinates first. Every number is bound, checked finite
and on the globe, and refused with a sentence otherwise.

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
the server and both renderers place what they are given. That includes the
words: a bar's name beside it and its value at its end, the categories under a
line or a column, a combo's second scale down the right in its measure's
colour, a heatmap's figures in its cells, a donut's whole and a gauge's reading
set large in the middle, a treemap's groups across the tops of their frames. A
chart laid out in rows — bars, a funnel, a heatmap — is as tall as its rows, so
two bars are two bars' height rather than stretched across a fixed box. A
layout of charts and no table is a legitimate paginated output. A `map` prints
too, in its own proportions and without its basemap — see "Maps" below. It used
to print a line saying to open the report in a browser.

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
| `column` | `x` dimension, `y` measure | Vertical columns against a scale |
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
| `radar` | `x` dimension, `y` measure | A spoke per category, a shape per series |
| `bullet` | `y` measure, plus `target:`; `x` optional | A bar per category against its target |
| `histogram` | `x` number; `y` measure optional; `bins:` | How a number's rows spread across its range |
| `boxplot` | `y` number; `x` dimension optional | Each category's quartiles, median and whiskers |
| `sankey` | `x` + `series` dimensions, `y` | A band from each category of `x` to each of `series` |
| `sunburst` | `x` + `series` dimensions, `y` | `series` as an inner ring, its parts of `x` around it |
| `calendar` | `x` date, `y` measure | A year a block of weeks, each day shaded |

`series:` names a second dimension to split by, and `stacked: true` stacks the
result — on `bar`, `column` and `area` only, because a stacked line has a top
edge that reads as a total nobody measured. `stacked: percent` stacks each
bucket to its whole, every part drawn as its share of the bucket against a
scale of percentages — on `bar` and `column`; an area stacked to its whole is
refused for now. The values stay the values, and hiding a series with its
legend key redraws the rest to the whole. Splitting a `pie` is refused for the
same class of reason as stacking a line: it is already part-to-whole.

A `column` chart is the bar chart stood up: for categories read left to right,
it has a scale down its left and each column's figure over its top while its
band has room for it. A `radar` needs three categories or more — with two it
is a line — and is for a profile across a handful of measures of one kind; its
rings are at the scale's ticks, and it cannot stack.

A `bullet` chart is a gauge's reading in a row's height: each category of `x`
is a bar along one shared scale, across a track shaded by how far it is to its
`target:` — a field or a fixed `value`, as a gauge's — with the target a mark
across it and "1,500 of 1,000 · 150%" beside it. Without `x` it is one bullet
for the whole set. `bands:` are where the track's shade changes, as fractions
of each row's target, ascending, at most two: `[0.6, 0.9]`, the default,
shades below 60% darkest, then to 90%, then the rest.

A `histogram` and a `boxplot` draw how a number spreads rather than what it
sums to, so both read it row by row — in the database, which is where the rows
stay: a histogram of four million invoices sends twelve bins, a box plot eight
numbers a category. A `histogram` bins `x`, which must be a number, into about
`bins:` bins (twelve unless it says, at most sixty) on round numbers — 500 to
1,000 rather than 487.3 to 974.6 — and counts the rows in each, or folds `y`
over them where it names one. A `boxplot` draws, for each category of `x` or
once for every row, the middle half of `y` as a box, its median across it, and
whiskers to the furthest rows inside one and a half interquartile ranges of
the box; the rows beyond are counted in the tooltip. Its `y` takes no
aggregate: the spread of sums is not the spread of anything. Quartiles are
nearest-rank and the median is the mean of the middle two, the same in every
database cronos reads, because they are worked out from row ranks rather than
from a percentile function only some of them have.

A `sankey` draws where a measure goes: each category of `x` a node down the
left, each of `series` a node down the right, and a band between every pair
that carries anything, as thick as its share of the whole. The server lays it
out — the bands meet their nodes edge to edge, and no two cross inside a node —
so a page and a screen draw the same picture. Each side draws its twelve
largest nodes and folds the rest into "Other". A `sunburst` is a treemap's
nesting as rings: `series` around the middle and each one's categories of `x`
around it, largest first, with the whole in the middle. Both need `series`.

A `calendar` draws a measure per day of `x`, which must be a date — a year a
block of weeks, Monday at the top, each day shaded by the quantile it falls in
and every day with nothing in it an outline. Its `x` takes no grain but `day`:
a month of days is one bucket of a column chart.

A `stat` takes a `trend:` — a date and a period — and draws its number over
those periods under the figure, with the last against the one before as the
change it reports: "▼ 12.4% vs Jun 2026". Whether a fall is good news is the
measure's to say, and `better: lower` says it, for days to pay or tickets
open. The periods are the same statement a line chart of the measure would
be, over the same rows the tile's own figure reads.

```yaml
- kind: stat
  label: Billed
  value: {field: total, aggregate: sum}
  trend: {field: issued_at, grain: month}

- kind: chart
  chart: calendar
  title: Billed by day
  x: {field: issued_at}
  y: {field: total, aggregate: sum}
```

```yaml
- kind: chart
  chart: sankey
  title: Where billing stands, by customer
  x: {field: customer_name}
  series: {field: status}
  y: {field: total, aggregate: sum}
```

```yaml
- kind: chart
  chart: histogram
  title: Invoice sizes
  x: {field: total}
  bins: 20

- kind: chart
  chart: boxplot
  title: Invoice sizes by status
  x: {field: status}
  y: {field: total}
```

```yaml
- kind: chart
  chart: bullet
  title: Billed against plan
  x: {field: region}
  y: {field: total, aggregate: sum}
  target: {value: 250000, label: Plan}
  bands: [0.5, 0.8]
```

Every chart is drawn at the width of the panel it is in, and drawn again when
that changes, so a circle stays round, a label is measured and cut to its room
rather than run off the edge, and category labels thin to every second or
third when they would touch. A bar carries its number at its end, and a bar
below zero grows left from a line at nothing. A chart split by `series:` has a
legend whose keys hide and show each series. A `donut` carries the total in its
middle, and both share charts list every slice with its value and its share.
Numbers on a chart are always the ones the server formatted.

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
and an accident. The viewer marks such a track "(right)" in the legend and
reads it against a second axis on the right, its numbers in the track's own
colour, because a reader otherwise has no way to know that line is not
comparable to the bars beside it.

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
nothing a bar is on. Each column carries its change written over it, and a
dashed thread takes the running total from one column to the next.

A **heatmap** needs both dimensions: `x` along the top and `series` down the
side. The grid comes back dense, and a pair no row matched is marked `empty`
rather than dropped — "no rows" and "rows totalling nearly nothing" are
different answers, and the ramp's lightest step cannot say both. A cell carries
its value where the value fits, and a key under the grid says what each shade
spans.

A **gauge** folds the whole set to one number and reads it against a `target:`,
which is either a measure or a fixed value:

```yaml
- kind: chart
  chart: gauge
  title: Billed against plan
  y: {field: total, aggregate: sum}
  target: {value: 100000, label: Plan}    # or {field: quota, aggregate: sum}
```

The dial runs from nothing to the target, both ends labelled, with the value and
its share of the target in the middle. The arc is capped at the target and
beating it turns it to the good colour and is said in words beside it, because
an arc drawn to 180% wraps past its own start and reads as 80%.

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
    layers: [polygon, scatter]
    geometry: region_shape      # a field holding GeoJSON
    lat: depot_lat
    lon: depot_lon
    basemap: {provider: mapbox, style: light}
```

| Layer | Needs | Draws |
| :--- | :--- | :--- |
| `polygon` | `geometry` | Regions shaded by `y` — a choropleth |
| `line` | `geometry` | Routes, roads and pipelines, coloured by `y` |
| `hexbin` | `lat`, `lon` | Points folded into hexagons, each shaded by the total inside it |
| `heat` | `lat`, `lon` | Point values spread into a density field |
| `cluster` | `lat`, `lon` | Points that would overlap drawn as one counted circle, separating as the reader zooms in |
| `bubble` | `lat`, `lon` | A circle per point, sized by `y` |
| `scatter` | `lat`, `lon` | A dot per point |
| `flow` | `lat`, `lon`, `toLat`, `toLon` | An arc from each origin to its destination, with a head where it lands |
| `radius` | `lat`, `lon`, `radiusKm` | A circle of `radiusKm` around each point, measured on the ground — a delivery area, a catchment |
| `h3` | `h3`, or `lat` and `lon` | Uber's H3 cells, each shaded by the total in it: a column of cell ids, or the places binned |

`x` names what each row is, in its tooltip — the region a polygon shades, the
depot a dot marks. `map.region` overrides it, so a block can group by a code and
label with a name.

Geometry is a **field**, not a bundled atlas: a column of GeoJSON, which is what
`ST_AsGeoJSON` returns in PostGIS and in DuckDB's spatial extension. There is no
world map to keep up to date and no list of the countries cronos knows about.
One field serves both geometry layers: the polygon layer shades the Polygons
and MultiPolygons in it and the line layer strokes the LineStrings and
MultiLineStrings, so a column holding districts and the roads between them is
one block. A geometry no requested layer draws — a line under a polygon layer —
is refused with a sentence rather than drawn as nothing. Points belong in `lat`
and `lon`: `ST_Y(geom) AS lat, ST_X(geom) AS lon` in the dataset's query.

An `h3` layer draws the cells of [H3](https://h3geo.org), the hexagon grid a
warehouse can index its rows by and join to anything else indexed the same way.
Given `h3`, a field of cell ids — as text, `871969c9bffffff`, or as the integer
some warehouses store — it shades each cell by its rows, as the warehouse
grouped them; with `h3Resolution` coarser than the column it adds each cell up
into its parent, as H3 itself rolls cells up. Without `h3` it bins the places
at `lat` and `lon` into cells at `h3Resolution`, 1 to 15, or at the resolution
about two dozen cells span the data. A value in the column that is not a cell
is left out, as a place with no coordinates is.

Every other layer of a large map is gathered in the database. H3 has no form in
the SQL most warehouses speak, so an h3 layer over more rows than a map holds
reads them — once, keeping the cells and never the rows — up to 200,000, and
says so under the map past that; index the rows by cell in the warehouse to
count them all. Adding cells up — binning places, rolling cells into parents,
or a cell's rows under more than one label — refuses `avg`, as hexagons do.

`hexbin` sizes its hexagons with `hexKm`, their width flat side to flat side in
kilometres; without it about two dozen span the data. A width rather than a
count, because a count redraws the grid whenever a filter moves the edge of the
data, and a hexagon that changes size when somebody filters by carrier is a
number that cannot be compared with the one before it. The grid is regular in
Web Mercator, which stretches a kilometre further from the equator, so the width
holds to within two and a half percent: it is set by the stretch at the middle
of the data rounded to a step of five percent, and a filter leaves the grid
exactly where it was unless it moves the data far enough to change that.

`labels: true` names the regions and places on the map itself — a region in
its middle, a place beside its mark — as many as fit without one covering
another, the largest first, so a continent names its biggest few and a street
names everything. `animate: true` moves a flow layer's arcs from where each
starts to where it lands; it holds still for a reader whose system asks for
less motion. A `radius` layer's circles are measured on the ground, so one far
north is taller on the map than one at the equator, as the ground it covers
is; a large map circles its places once the reader has zoomed to them one by
one, since a circle around a crowd is an area nobody serves.

**Shades.** The polygon, line and hexbin layers shade each mark from a ramp
of six. `classify` says how the values are split among them:

| `classify` | Each shade holds | For |
| :--- | :--- | :--- |
| `quantile` (default) | the same number of places | skewed measures — most of them |
| `equal` | the same width of values | a measure whose steps mean something: a percentage, a score |
| `jenks` | a run of values between the widest gaps | clumped data; natural breaks, found exactly rather than approximated |
| `custom` | the classes `breaks` sets: up to five upper bounds, ascending | a regulator's thresholds, last year's bands |

A value on a break is in the shade below it. A custom class keeps its colour
whatever the data is, so two reports shade the same threshold alike, and a
band no value reaches is left out of the legend.

`ramp: diverging` takes two hues either side of `midpoint` — zero unless set —
for a measure whose middle means something: a change on last year, a margin,
performance against a target. The three shades below the middle and the three
above are classed apart, each side palest nearest it; classed together, one
large rise would put every fall and every small rise in the same pale band. The
hues are blue and orange, which hold apart under every common colour-vision
deficiency, as red and green do not.

```yaml
map:
  layers: [polygon]
  geometry: shape
  ramp: diverging
  midpoint: 95
  classify: jenks
```

A diverging ramp's custom breaks include its midpoint and at most two either
side of it. `classify` and `ramp` on a map with no shaded layer are refused:
there is nothing for them to colour.

**Colour by category** with `series`, on the layers whose colour is otherwise
unspent — `scatter`, `bubble`, `cluster` and `flow`:

```yaml
- kind: chart
  chart: map
  title: Every delivery, by carrier
  x: {field: city}
  y: {field: parcels, aggregate: sum}
  series: {field: carrier}
  map: {layers: [cluster], lat: lat, lon: lon}
```

It is refused beside `polygon`, `line`, `hexbin` or `heat`, which already spend
colour on the value: two meanings for one channel is how a choropleth with
coloured dots on it becomes unreadable. Categories take the first three colours
of the palette, the way a scatter's series do, and a fourth and later share the
third, named together in the legend. Each is drawn in a shape of its own as
well — a circle, a square, a triangle, each at the same area — on screen and on
paper alike, so a reader who cannot tell two of the colours apart can still
tell the categories apart, and the key shows the shape beside the colour.

A `bubble` layer keys its sizes under the map: three rings drawn to the
bubbles' own rule, at round numbers up to the largest value on it.

The server projects every geometry to **Web Mercator**, simplifies it, and sends
SVG paths — the same argument that keeps currency formatting on the server.
Shipping rings and a projection to every one of an ISV's end users would cost
more than the whole embed bundle's budget. Web Mercator specifically, because it
is the projection every XYZ tile server already publishes in, so a basemap lines
up with the data for free — and because the viewBox of that SVG is then the
window onto the world, a reader **pans and zooms** it without a map library:
drag to move, ⌘ or Ctrl and scroll (or a trackpad pinch) to zoom about the
cursor, two fingers on a touch screen, the `+`, `−` and fit buttons, or the
keyboard once the map has focus. A plain scroll wheel scrolls the page and says
how to zoom; one finger scrolls a phone's page past the map. A map embedded in
somebody else's page must not take their scroll wheel away from them.

#### Basemaps

`basemap` is **empty by default and opt-in**. A basemap is a request from the
reader's browser to a third party that cronos would have chosen for them; it
discloses roughly where the data is, and each provider has terms. Name one of
the providers cronos knows the terms of, or give a URL for anything else:

| `provider` | `style` | Key |
| :--- | :--- | :--- |
| `openstreetmap` | `standard` | None. The OpenStreetMap Foundation's [tile usage policy](https://operations.osmfoundation.org/policies/tiles/) forbids heavy use: right for a demo or a low-traffic internal report, wrong for an embedded product at volume. |
| `mapbox` | `streets` (default), `outdoors`, `light`, `dark`, `satellite`, `satellite-streets`, `navigation-day`, `navigation-night`, `auto`, or a Mapbox Studio style as `owner/style` | A **public** token (`pk.`) with the `styles:tiles` scope, from `${secret:mapbox-token}`. A secret `sk.` token is refused rather than sent to every reader's browser. |
| `google` | `roadmap` (default), `satellite`, `terrain`, `hybrid`, `dark` (the road map in night colours), `auto`; `language` and `region` localise the labels | A key with the Map Tiles API enabled, from `${secret:google-maps-key}`. |

```yaml
basemap: {provider: openstreetmap}
basemap: {provider: mapbox, style: dark}
basemap: {provider: google, style: hybrid, language: id, region: ID}
basemap: {provider: mapbox, style: acme/ckx1y2z3, key: "${secret:mapbox-acme}"}
basemap:
  url: https://tiles.example.org/{z}/{x}/{y}{r}.png?key=${secret:tiles-example}
  attribution: © Example Maps
  maxZoom: 18
```

`style: auto` follows the page the map is drawn on: the provider's light map
on a light page and its dark one on a dark page — Mapbox's `light` and `dark`,
Google's `roadmap` and `dark`. The theme is the one a `data-theme` attribute on
the page or the embedded element names, and otherwise the reader's system
setting; a reader who switches it gets the other map the next time the map
moves. OpenStreetMap's tiles have one look, so it has no `auto`.

A provider brings its own credit line and logo, in its own wording and with the
links its terms want, and cronos draws them; `attribution` is refused beside a
provider for that reason. Google's copyright names whoever supplied the imagery
in view, so the viewer asks Google for it again as the reader pans. `maxZoom`
caps how deep a reader can zoom, below the provider's own limit — a cost control,
since every level is four times the tiles.

A **URL** is any XYZ template: `https`, because an embedded report is served
over https and a browser blocks mixed-content tiles silently; `{z}`, `{x}` and
`{y}`; and `{r}` where the server has high-density tiles, filled with `@2x` on a
screen that has the pixels. `attribution` is required with one, because every
tile source worth using requires its credit line be displayed.

**Keys are secrets, never literals**, resolved the way a datasource's are — see
"Secrets" in [deploying.md](deploying.md). A basemap's key reaches every
reader's browser by construction: it is in every tile request, and anybody who
opens the network tab has it. So a basemap may only name a secret made to be a
tile key — `mapbox-…` for Mapbox, `google-…` for Google, `tiles-…` in a URL —
and anything else is refused when the report is saved. Without that rule,
`${secret:warehouse-password}` in a tile URL would send the database password to
every reader. A deployment with no key for a provider draws the map without its
basemap, says so under the map in words that name no setting, and logs which
secret to set. The builder shows whether a map's key is set and takes one on the
spot, stored in the project like any other secret; Settings → Secrets lists
every key the project's maps use.

Google's terms forbid its maps **beside another provider's** on one screen
(Maps Service Terms 3.2.3(e)), so an output that draws a Google basemap and any
other basemap is refused; a map with no basemap is not somebody else's map. Give
the Google maps a report of their own, as the demo's
`parcel-network-google` does.

On **paper** a map is drawn in its own proportions, 90mm tall or as wide as
the page allows — polygons, lines, hexagons, dots and arcs as vector marks, with
the legend — and without its basemap. Its places are sized against its height,
so a map of somewhere tall prints them as large as one of somewhere wide. The
tiles are a third party's, requested by a reader's browser under that party's
terms, and none of those terms is a server printing them into a document that is
then mailed to five thousand people. A ring inside another is a hole, on paper
and on screen alike, whichever way the data wound it; an island in a lake, and
one region's exclave in another's hole, are drawn after the hole they sit in.

#### Maps that filter

A map is also a control. Where the report has a filter a map can set, the map
sets it, and the rest of the report follows:

- **Click a place to narrow the report to it.** A map whose places are labelled
  by a text field — `x`, or `map.region` — sets the `string` or `enum` filter
  bound to that same field in its dataset. A click narrows to that place, and a
  click on it again lets it go. The map itself stays whole, with the place
  picked marked, the way a dropdown lists every value rather than only the
  chosen one: narrowed by its own pick it would show a single place, and no way
  to choose another without letting go of the first. On paper, where nothing
  can be clicked, it is narrowed like everything else.
- **Filter to this view.** A map whose `lat` and `lon` are the fields an `area`
  filter binds in its dataset offers to narrow the report to the part of the
  world in view, and outlines the area applied — a box, or a circle for `near`
  — with the way back, "Show everywhere".

Nothing is declared on the map for either: the filters and their bindings say
it already. The filter bar shows what a map set, and applying the bar for
something else keeps it — a picked region stays `in`, rather than becoming the
`contains` a text box would send. An embedding host hears of it as a
`cronos:filter` event (`onFilter` in React), so a host that keeps the filters
itself does not fall behind.

A filter a map sets is a reader's choice and never a constraint on them: a host
pins parameters, not filters, and row-level security is applied under every
query whatever its filters. A map left unnarrowed by its own pick shows no row
its reader could not already see.

#### One dataset over another

`overlays` draws other datasets over a map — depots over the deliveries around
them, stores over the catchments they serve. Each is a map block of its own,
without `kind` and `chart`, reading its own dataset (the map's when it names
none) through its own query; filters and row-level security apply to each as
they do to any block:

```yaml
map:
  layers: [heat]
  lat: lat
  lon: lon
  overlays:
    - dataset: depots
      title: Depots
      x: {field: city}
      y: {field: capacity, aggregate: sum}
      map: {layers: [scatter], lat: lat, lon: lon}
```

Up to four, drawn over the map in the order listed, and the map opens on a box
that holds all of them. Places with no category are drawn in a colour of their
own and named under the map beside it; places coloured by category, or shapes
shaded, are named over their own legend. An overlay has no basemap — it is
drawn over the map's — and no overlays of its own. A large one is gathered and
refined as the reader zooms, like any map; it never sets the report's filters,
which is the map's to do.

#### Maps that play through time

`time` plays a map through the periods of a date — each day's deliveries, a
region's revenue month by month — a period at a time:

```yaml
map:
  layers: [bubble]
  lat: lat
  lon: lon
  time: {field: dropped_on, grain: day}
```

The grain is `day`, `week`, `month`, `quarter` or `year`, and every period is
played, however many there are. The map opens on every period together, as
paper prints it, with a slider under it: play, drag, or step through the
periods with the arrow keys, and "All periods" goes back. In a period each
place is its value then, sized against the busiest place of any period, and
each region is shaded from one set of shades for all of them — so a colour or a
bubble means the same number in the first period and the last, and the legend
says what the shades mean while a period is shown. A region with nothing in a
period is drawn empty and says so; a place or a flow with nothing in it is not
drawn at all. A row with no date counts in the map as it opens, and in no
period.

The map as it opens adds each place up across its periods, and a `size`
measure with it, so `time` refuses `avg` as a map that folds regions from
points does. It is refused beside `hexbin` and `h3`, which fold every place at
once — give them a block of their own — and on an overlay, which is drawn over
the map as it is. A place in each of its periods is a row, so the 5,000 rows a
map holds are places *and* periods: a busy map by day can say it is partial
where the same map by week would not.

#### A million places

A map with more places than a browser can hold is not cut short. Up to 5,000
places a map is its places, a mark each, exactly as it always was. Past that it
is asked again, differently, and the database does the gathering — over every
row, so no total on it counts only some of them:

- **Dots, bubbles, heat and clusters** are gathered into a grid a few pixels
  wide at the depth the map opens at. Each time a reader stops moving the map
  it asks for the part in view, a finer grid each time, until every cell is one
  place with its own name and value. A cell of several says how many places it
  holds and their total — the measure's own aggregate applied again, or their
  average where the measure is an average, which its tooltip says.
- **Hexagons** are folded in the database: the same hexagons, holding the same
  places, as a small map would draw.
- **Regions** under points are totalled over their own rows, so an average of
  a region is exact rather than refused.
- **Flows** are gathered into routes between cells, the busiest 5,000 kept,
  and split into the flows they are as a reader zooms in.
- **Categories** keep their colours in every view. The most common get the
  palette's first colours once, when the map opens, and every view is coloured
  by that.

A view is a render of less of the world: the same parameters, filters and
row-level security, applied again by the server — an embedded reader's view is
asked for with the embed's own token. The map says under itself how many places
it holds. On paper it is its cells, dots sized by how many places each holds; a
page cannot zoom, so a few thousand of them.

How long a large map takes to open is how long the warehouse takes to group
every row: a million places open in about 0.9 s on Postgres and 3.4 s on SQLite,
and a view of a few streets answers in tens of milliseconds. The demo's `fleet`
report draws two hundred thousand van positions three ways.

Regions are the one thing a map still cuts: one reads at most 5,000 of them,
and says so under itself and on paper if it had more.

One block compiles to one query, so a map's layers share a grain. Asking for
the geometry field *and* points runs at the point grain and adds each region up
from the points under it, and a `hexbin` layer adds each hexagon up from the
points inside it. That is exact for `sum`, `count`, `min` and `max`, and it is
refused for `avg`: an average of averages is a number nobody measured that looks
entirely plausible. `hexbin` is refused beside `polygon` and `line` for the same
family of reason: both shade from the ramp, one from rows and one from folded
points, and one legend cannot explain two scales. Split the layers across two
blocks if you need both. Two tables become one map by joining them in the
dataset's query — the demo's `zones` dataset puts a zone's outline and its
depot's position in one row — or by drawing one over the other with
`overlays`, each at its own grain.

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
