# Migrating from JasperReports

JasperReports Server Community Edition was discontinued on 25 January 2024. This
is the path off it for a team whose reports are `.jrxml` files.

```sh
# Read the estate and report what would happen. Writes nothing.
go run ./cmd/cronos-import ./jasper-reports

# Do it.
go run ./cmd/cronos-import -datasource warehouse -folder /finance \
  -out ./definitions ./jasper-reports
```

There is a report to try it on in `examples/jasper/`, over the demo warehouse,
so the output can be read before pointing it at anything real:

```sh
go run ./cmd/cronos-import examples/jasper
```

`cronos-import` ships in both release channels, so an estate does not need a Go
toolchain to move. From a release archive:

```sh
tar xzf cronos_v1.0.0_linux_amd64.tar.gz
./cronos_v1.0.0_linux_amd64/cronos-import -datasource warehouse \
  -out ./definitions ./jasper
```

Or from the image, if that is what the deployment runs:

```sh
docker run --rm -v "$PWD/jasper:/jasper:ro" -v "$PWD/definitions:/out" \
  cronos:latest cronos-import -datasource warehouse -out /out /jasper
```

The importer covers the common shape of an operational report — a query, its
parameters, a grouped and subtotalled table on paper — and says, per file, what
it could not carry. Everything below is what "covers" means, because the useful
thing to know before migrating four hundred reports is where the work will be.

## What the two formats disagree about

Everything else follows from this, so it is worth two paragraphs.

**JasperReports is a pixel layout.** Elements sit at (x, y) inside a band, and
the document is what those coordinates draw. **cronos is semantic.** A table
block groups and subtotals and the renderer decides where that lands on paper.
There is no function from the first to the second, because the first does not
record what a column *is* — only where it was.

So the importer carries meaning and abandons appearance. It reads the query, the
parameters, the fields, the groups, the subtotals and the page setup, and it
*infers* a table from the fact that a detail band's text fields in reading order
are, in every tabular Jasper report ever written, the columns. It does not carry
fonts, colours, borders, pixel positions or Java expressions.

The second disagreement is smaller and more useful. A `.jrxml` embeds its query;
cronos separates the governed query from the thing that draws it — see
[report-format.md](report-format.md) for why. So **one `.jrxml` becomes two
definitions**: a `Dataset` and a `Report`.

## What comes across

| In the `.jrxml` | Becomes | Notes |
| :--- | :--- | :--- |
| `<queryString>` | `Dataset.query` | `$P{X}` becomes `{{ .params.x }}`, a bound argument |
| `<parameter>` | `Dataset.params` | Java class → type; `new Date()` → `default: today` |
| `<field>` | `Dataset.fields` | Java class → type; role from the report's own totals |
| `<variable calculation="Sum">` over `$F{x}` | `x` is a measure that sums | The report telling us what a quantity is |
| `<group>` + `isStartNewPage` | `groupBy` + `pageBreak: perGroup` | The outermost group only |
| `<variable resetType="Group">` | `subtotals` | Per-group totals |
| `<detail>` text fields | table `columns`, in page order | Read through frames, in reading order |
| `<columnHeader>` static texts | field `label` | Matched to columns by horizontal overlap |
| `<sortField>` | table `sort` | |
| `pageWidth`/`pageHeight`/margins | `page` | 595×842 points is recognised as A4 |
| `$V{PAGE_NUMBER}` in the page footer | `footer.text` | "Page {{ .page }} of {{ .pages }}" |
| `<title>` first static text | a `text` block, and the report's title | |
| `<barChart>` `<lineChart>` `<areaChart>` | a `chart` block | Category → x, value → y |
| A report-level total that was printed | a `stat` block | Labelled with the caption beside it |
| `pattern="¤#,##0.00"` | `format: currency` | The symbol is the author saying it is money |

Each file also gains an **`interactive` output** it never had: the same columns
and charts without the pagination. That is the point of moving — the report you
have been mailing as a PDF becomes embeddable and filterable from the same
definition.

## What does not, and what to do instead

| Not carried | Why | The usual translation |
| :--- | :--- | :--- |
| **Subreports** | cronos has no nested report | A second dataset and a second block, or a join in this query |
| **Crosstabs** | No pivot block in v1 | A query grouping by both axes |
| **Java expressions in a column** | No computed column | Put the expression in the dataset's `SELECT` |
| **Scriptlets** | No code execution in a definition | Usually becomes SQL |
| **Images and logos** | Not a block | The paginated output's `header.template`, a Typst file |
| **`printWhenExpression`** | No conditional block | Imported unconditionally — check it does not now show what it hid |
| **Fonts, colours, borders, positions** | Styled by theme, laid out by block | Nothing; this is the trade |
| **Pie charts** | No pie renderer | Imported as a bar chart of the same values |
| **The JasperReports 7 element syntax** | This reads the classic `<textField>` bands | The query still imports; the layout has to be rebuilt |
| **XY, time-series, scatter, gauge charts** | cronos charts a dimension against a measure | Rebuild in the builder |
| **Row-level security** | A `.jrxml` has none to carry | **Read the next section.** |

### Two files are refused outright

Refused, rather than imported and wrong:

- **`$P!{column}` in a query.** This is how a Jasper report makes its `ORDER BY`
  configurable, and it works by concatenating the caller's string into the SQL.
  cronos binds parameters and has deliberately no path from a value to query
  structure. Binding it would silently order by a constant; passing it through
  would carry an injection into a system built to make it impossible. Declare
  the alternatives as an `enum` parameter with one fixed SQL fragment per value,
  or split the report per variant.
- **A query that is not SQL.** HQL, MDX and XPath are not SQL with different
  keywords. PL/SQL means an Oracle database, which cronos has no driver for.

## Read this before you publish

**An imported dataset has no row-level security, and nothing in the file could
have given it any.** JasperReports Server enforced access at the report level and
by passing a parameter into the query, so a `.jrxml` carries no statement of
which rows belong to whom. Every imported dataset therefore returns every row its
query selects, to anyone who can reach the report.

The importer says so, once per file. Before publishing anything an end customer
can reach, add the predicate:

```yaml
rowLevelSecurity:
  - predicate: customer_id = {{ .scope.customer_id }}
```

See [tenancy.md](tenancy.md). This is the one finding that is a security decision
rather than a missing feature, and the only one worth stopping for.

## Reading the output

Nothing is written without `-out`. The default run prints the work list:

```
jasper/auftrag.jrxml
  review: crosstab — a crosstab pivots rows into columns; cronos v1 has no pivot
          block, so this is not imported — a query that groups by both axes is
          the closest equivalent
  review: image — an image is not carried; a logo or letterhead belongs in the
          paginated output's header template, which is a Typst file
  note: 12 cosmetic differences — -v lists them

412 files · would write 431 definitions · 39 reports share a dataset already imported · 61 to review · 4 blocked

Nothing was written. Pass -out <dir> to write the definitions.

4 files need a person before they are migrated.

! 392 datasets have no row-level security.
  A .jrxml carries none — JasperReports Server enforced access above the
  report — so each of these returns every row its query selects to anybody who
  can reach it. …
```

Three grades, and they are a triage order:

- **blocked** — the file produced no report, or was refused. Someone has to open
  it. This is the number that decides whether a migration is finished, and the
  command exits non-zero while it is above zero.
- **review** — it imported, and something changed or went missing. Worth a look.
- **note** — appearance. Expected, listed under `-v`, and safe to ignore.

Files with nothing above *note* are not printed at all. In a real estate that is
most of them, and a tool that printed a paragraph about each one would bury the
four that mattered.

### Nothing is dropped silently

The importer walks every element in the file and reports the ones it does not
carry — including ones it has never heard of, which come back as "a JasperReports
construct this importer does not read". That is deliberate: the alternative is an
importer that reports exactly what its author remembered, and JasperReports has
some two hundred elements across six versions plus a component namespace anyone
can extend.

## Things worth knowing

**Identical queries become one dataset.** Forty reports over one query import as
one `Dataset` and forty `Report`s bound to it, which is the arrangement the
format exists to make possible. `-share-datasets=false` turns it off.

**Field names may only be lower-cased.** A cronos field name reaches SQL as a
column reference, so `CustomerName` imports as `customername` — not
`customer_name`, which would name a column the query does not return. If the
query aliased it as `AS "CustomerName"` *with quotes*, Postgres will only match
that exact spelling and the import says so. Drop the quotes or alias in lower
case. Parameter names have no such constraint and become `from_date`.

**Numeric columns are guessed.** Nothing in a `.jrxml` says which numbers are
quantities. A column the report totalled is a measure; one the report grouped by
is a dimension; otherwise a numeric column whose name ends in `id`, `code`, `no`,
`year` and similar is a dimension, and anything else numeric becomes a measure
that sums. The count of guesses is reported. Check them.

**Non-UTF-8 files are read.** Jasper Studio wrote the platform default encoding
for years, so ISO-8859-x and windows-125x files are common and are decoded
properly. A customer called Müller stays Müller.

**Re-running is safe.** An import over its own output changes nothing. A
definition that exists and *differs* stops the run rather than being overwritten;
`-force` overrides that.

## After the import

1. **Write the `DataSource`.** Every imported dataset reads `-datasource`, and a
   `.jrxml` names no database — no driver, no host, no credential — so the
   importer cannot write it. It notices the definition is absent from the output
   directory and prints the one to fill in. See
   [report-format.md](report-format.md).
2. Add row-level security to anything an end customer can reach.
3. Work the **blocked** list, then the **review** list.
4. Publish, and check a rendered PDF against one Jasper produced. The numbers
   should match exactly; the layout will not.
