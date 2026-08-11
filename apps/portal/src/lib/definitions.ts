import { carryOver, document, fromYaml, toYaml, unmodelled, type Yaml } from './yaml'
import type { Field } from './types'

/**
 * Form state to a definition document.
 *
 * The one place the interface knows the file format, so a field renamed in the
 * spec is one change here rather than four across the forms — and so the shape
 * can be tested without a browser.
 *
 * These functions are deliberately dumb. Everything worth validating is
 * validated by the server, which compiles every block before storing anything;
 * a second implementation of those rules here would be a second thing to keep
 * in step and the one that drifted would be the one an author trusted.
 */

/** What the portal's source picker calls a kind, and what the format calls it. */
const DRIVERS: Record<string, string> = {
  postgres: 'postgres',
  mysql: 'mysql',
  clickhouse: 'postgres', // speaks the same wire dialect as far as we compile
  bigquery: 'postgres',
  objectstore: 'object-store',
  excel: 'object-store',
  api: 'object-store',
}

export interface SourceInput {
  name: string
  slug: string
  kind: string
  host?: string
  port?: number
  database?: string
  user?: string
  uri?: string
  filePath?: string
  /**
   * The connection string exactly as stored.
   *
   * Set only when reading an existing source, and only honoured while the
   * connection fields are untouched. Not every DSN decomposes into host, port
   * and database — `file:cronos-demo?mode=memory&cache=shared` is a real one
   * in this repository — so rebuilding one from parts that were never parsed
   * out of it produces a connection string to nowhere.
   */
  dsn?: string
  /* The limits, kept as read. The form does not ask for them, and defaulting
     an existing source back to a million rows would raise a ceiling somebody
     lowered deliberately. */
  maxRows?: number
  statementTimeout?: string
}

/**
 * A datasource.
 *
 * The password is not here and never will be. A definition is a file somebody
 * commits; a secret in one is a secret in their git history for ever. The
 * format's answer is `${secret:name}`, resolved where the connection is opened.
 */
export function dataSource(input: SourceInput): string {
  const driver = DRIVERS[input.kind] ?? input.kind
  const spec: Record<string, Yaml> = { driver }

  if (driver === 'object-store') {
    spec.uri = input.uri || input.filePath
    spec.format = input.kind === 'excel' ? 'csv' : 'parquet'
  } else {
    spec.dsn = input.dsn || dsn(driver, input)
  }
  spec.limits = {
    statementTimeout: input.statementTimeout ?? '30s',
    maxRows: input.maxRows ?? 1000000,
  }

  return document('DataSource', { name: input.slug, title: label(input) }, spec)
}

/**
 * A connection string with the password left as a reference.
 *
 * Written out rather than assembled from the parts at read time, because the
 * format takes one DSN and an operator reading the file should see the shape
 * of what will be connected to.
 */
function dsn(driver: string, input: SourceInput): string {
  const user = input.user || 'cronos'
  const host = input.host || 'localhost'
  const port = input.port ? `:${input.port}` : ''
  return `${driver}://${user}:\${secret:${input.slug}_password}@${host}${port}/${input.database ?? ''}`
}

export interface DatasetInput {
  name: string
  slug: string
  description?: string
  source: string
  query: string
  fields: Field[]
  /**
   * The row-scope predicate, as the author wrote it.
   *
   * Taken whole rather than built from a field name: the form asks for a
   * predicate and the format stores one, and generating `x = {{ .scope.x }}`
   * from a column would quietly refuse every scope that is not an equality.
   */
  predicate?: string
}

/** A dataset. */
export function dataset(input: DatasetInput): string {
  const spec: Record<string, Yaml> = {
    sources: [{ ref: input.source }],
    query: ensureTrailingNewline(input.query),
    fields: input.fields.map(field),
  }
  if (input.predicate?.trim()) {
    // Row scope is the difference between a dataset an embedded end customer
    // may read and one only the project may. Nothing else in the file says
    // which it is.
    spec.rowLevelSecurity = [{ predicate: input.predicate.trim() }]
  }
  return document('Dataset',
    { name: input.slug, title: label(input), description: input.description }, spec)
}

function field(f: Field): Yaml {
  return {
    name: f.name,
    type: f.type,
    role: f.role,
    label: f.label || undefined,
    hidden: f.hidden || undefined,
    // Sum, because the field editor does not ask and a measure with no
    // aggregate is refused on save. It is the right default for money and the
    // wrong one for a rate, so the editor should offer the choice — until it
    // does, an author changes one line in the file rather than being unable to
    // save at all.
    aggregate: f.role === 'measure' ? 'sum' : undefined,
    format: f.format === 'preformatted' ? undefined : f.format,
  }
}

export interface ReportInput {
  name: string
  slug: string
  description?: string
  folder?: string
  dataset: string
  blocks: ReportBlockInput[]
  /**
   * The output profile the blocks belong to.
   *
   * Defaults to the interactive one, which is what the builder draws. Carried
   * when reading a report back so that opening a paginated profile and saving
   * it does not relabel it as interactive — the same blocks, silently moved to
   * a different renderer, is how a monthly PDF stops being a PDF.
   */
  output?: { name: string; renderer: string; page?: Yaml }
}

export interface ReportBlockInput {
  /**
   * What the builder calls it: stat, bar, line or table.
   *
   * The format splits those into a kind and a chart type — `kind: chart` with
   * `chart: bar` — so that adding a line chart is a new value rather than a new
   * block kind every renderer has to learn. The builder's palette is a list of
   * things you can drop on a canvas, which is a different question, and
   * translating between them is this file's job.
   */
  kind: string
  title?: string
  dataset?: string
  field?: string
  aggregate?: string
  groupBy?: string
  grain?: string
  chart?: string
  columns?: string[]
  pageSize?: number
}

/**
 * A report.
 *
 * One interactive output, because that is what the builder draws. A paginated
 * profile is a different layout of the same numbers and wants its own canvas —
 * emitting a guess at one from this canvas would produce a PDF nobody designed.
 */
export function report(input: ReportInput): string {
  const spec: Record<string, Yaml> = {
    dataset: input.dataset,
    outputs: [{
      name: input.output?.name ?? 'interactive',
      renderer: input.output?.renderer ?? 'interactive',
      // Paper size and margins. Inside a list the builder rewrites, so
      // carry-over cannot reach it and it is read back instead.
      page: input.output?.page,
      layout: input.blocks.map(block),
    }],
  }
  return document('Report',
    { name: input.slug, title: label(input), description: input.description, folder: input.folder },
    spec)
}

/** The palette entries that are charts, and which chart each one is. */
const CHARTS: Record<string, string> = { bar: 'bar', line: 'line', area: 'area' }

function block(b: ReportBlockInput): Yaml {
  const reads = b.dataset || undefined

  if (b.kind === 'stat') {
    return {
      kind: 'stat', dataset: reads, label: b.title,
      value: { field: b.field, aggregate: b.aggregate ?? 'sum' },
    }
  }
  if (b.kind === 'table') {
    return {
      kind: 'table', dataset: reads, title: b.title,
      columns: b.columns ?? [], pageSize: b.pageSize || undefined,
    }
  }
  if (CHARTS[b.kind] || b.kind === 'chart') {
    return {
      kind: 'chart', dataset: reads, title: b.title,
      chart: CHARTS[b.kind] ?? b.chart ?? 'bar',
      x: { field: b.groupBy, grain: b.grain || undefined },
      y: { field: b.field, aggregate: b.aggregate ?? 'sum' },
    }
  }
  return { kind: b.kind, dataset: reads, title: b.title, text: b.title }
}

export interface ScheduleInput {
  name: string
  slug: string
  report: string
  output: string
  cron: string
  timezone: string
  burstDataset?: string
  recipientField?: string
  to: string
  subject?: string
  channel?: string
  /** The attachment's filename template, as stored. */
  filename?: string
  concurrency?: number
  retries?: number
  alert?: string
}

/** A schedule. */
export function schedule(input: ScheduleInput): string {
  const spec: Record<string, Yaml> = {
    report: input.report,
    output: input.output,
    cron: input.cron,
    timezone: input.timezone,
  }

  if (input.burstDataset && input.recipientField) {
    spec.burst = {
      over: { dataset: input.burstDataset },
      bind: { customer_id: `{{ .row.${input.recipientField} }}` },
      concurrency: input.concurrency || undefined,
    }
  }

  spec.deliver = [{
    via: input.channel ?? 'email',
    to: input.to,
    subject: input.subject || undefined,
    attach: { filename: input.filename || `${input.slug}-{{ .run.periodEnd }}.pdf` },
  }]

  if (input.retries || input.alert) {
    spec.onFailure = {
      retries: input.retries || undefined,
      backoff: input.retries ? 'exponential' : undefined,
      alert: input.alert || undefined,
    }
  }
  return document('Schedule', { name: input.slug, title: label(input) }, spec)
}

/**
 * The display name, or nothing when it says the same as the identifier.
 *
 * A form that has only ever been given a slug would otherwise write `title:
 * warehouse` beside `name: warehouse` on every save, which is a line the
 * author did not write and does not mean anything.
 */
function label(input: { name: string; slug: string }): string | undefined {
  return input.name && input.name !== input.slug ? input.name : undefined
}

/** A block scalar needs its last line terminated, like every other line. */
function ensureTrailingNewline(s: string): string {
  return s.endsWith('\n') ? s : s + '\n'
}

/* ------------------------------------------------------------------------ *
 * Reading a definition back into a form.
 *
 * Editing is publishing the same name again — the store upserts, and keeps the
 * old bytes addressable — so an edit path is a load path and nothing more. The
 * load is the honest half: a form models a subset of the format, so opening a
 * document in one and saving it would quietly drop whatever the form does not
 * show. Every reader therefore returns what it could not model alongside what
 * it could, and the form says so before anybody presses save.
 * ------------------------------------------------------------------------ */

/** A document, as a form sees it, plus what the form would drop on save. */
export interface Loaded<T> {
  input: T
  /** Paths present in the stored file that saving this form would not write. */
  drops: string[]
  /**
   * The stored document, kept so a save can fold back the keys the form does
   * not model. Opaque to the form: it is handed to `withCarry` and nothing
   * else ever reads it.
   */
  stored: Yaml
}

/** Reads the envelope and hands the spec to a reader. */
function load<T>(text: string, read: (meta: Doc, spec: Doc) => T, emit: (v: T) => string): Loaded<T> {
  const stored = asMap(fromYaml(text))
  const input = read(asMap(stored.metadata), asMap(stored.spec))
  // Drops are measured against what a save would actually write, carry-over
  // included — otherwise the warning would name keys that survive.
  const written = carryOver(fromYaml(emit(input)), stored)
  return { input, drops: unmodelled(stored, written), stored }
}

/**
 * A document about to be published, with the parts of the original the form
 * never showed folded back in.
 *
 * Creating passes nothing to carry, and this is the identity function.
 */
export function withCarry(yaml: string, from?: Loaded<unknown>): string {
  if (!from) return yaml
  return toYaml(carryOver(fromYaml(yaml), from.stored)) + '\n'
}

type Doc = { [key: string]: Yaml }

function asMap(v: Yaml): Doc {
  return v !== null && typeof v === 'object' && !Array.isArray(v) ? v : {}
}
function asList(v: Yaml): Yaml[] { return Array.isArray(v) ? v : [] }
function str(v: Yaml): string { return typeof v === 'string' ? v : v == null ? '' : String(v) }
function num(v: Yaml): number | undefined { return typeof v === 'number' ? v : undefined }

/** What the portal's picker calls a driver. Several kinds share one. */
const KINDS: Record<string, string> = { postgres: 'postgres', mysql: 'mysql', 'object-store': 'objectstore' }

/**
 * A datasource.
 *
 * The kind comes back as the driver, so a source the author created as
 * ClickHouse reopens as Postgres — the file only ever recorded the wire
 * dialect, and inventing the original from it would be a guess. The file is
 * what runs, so the file is what the form shows.
 */
export function readDataSource(text: string): Loaded<SourceInput> {
  return load(text, (meta, spec) => {
    const driver = str(spec.driver)
    const connection = str(spec.dsn)
    const limits = asMap(spec.limits)

    return {
      name: str(meta.title) || str(meta.name),
      slug: str(meta.name),
      kind: KINDS[driver] ?? driver,
      uri: spec.uri ? str(spec.uri) : undefined,
      // Before the parsed parts, which never include a dsn and so cannot
      // overwrite it.
      dsn: connection || undefined,
      maxRows: num(limits.maxRows),
      statementTimeout: str(limits.statementTimeout) || undefined,
      ...parseDsn(connection),
    }
  }, dataSource)
}

/** Pulls the pieces back out of a connection string, password included or not. */
function parseDsn(connection: string): Partial<SourceInput> {
  const m = /^[a-z0-9-]+:\/\/(?:([^:@/]*)(?::[^@]*)?@)?([^:/?]*)(?::(\d+))?(?:\/([^?]*))?/.exec(connection)
  if (!m) return {}
  return {
    user: m[1] || undefined,
    host: m[2] || undefined,
    port: m[3] ? Number(m[3]) : undefined,
    database: m[4] || undefined,
  }
}

/** A dataset. */
export function readDataset(text: string): Loaded<DatasetInput> {
  return load(text, (meta, spec) => ({
    name: str(meta.title) || str(meta.name),
    slug: str(meta.name),
    description: str(meta.description) || undefined,
    source: str(asMap(asList(spec.sources)[0]).ref),
    query: str(spec.query),
    fields: asList(spec.fields).map(readField),
    predicate: str(asMap(asList(spec.rowLevelSecurity)[0]).predicate) || undefined,
  }), dataset)
}

function readField(v: Yaml): Field {
  const f = asMap(v)
  return {
    name: str(f.name),
    type: str(f.type) as Field['type'],
    role: str(f.role) as Field['role'],
    label: str(f.label),
    hidden: f.hidden === true,
    // The writer omits `preformatted`, because it means "leave the value
    // alone" and the absence of a format says the same thing.
    format: (str(f.format) || 'preformatted') as Field['format'],
  }
}

/** A report. Blocks come back as the builder's palette names, not the file's. */
export function readReport(text: string): Loaded<ReportInput> {
  return load(text, (meta, spec) => {
    const outputs = asList(spec.outputs).map(asMap)
    const shown = outputs.find((o) => str(o.renderer) === 'interactive') ?? outputs[0] ?? {}
    return {
      name: str(meta.title) || str(meta.name),
      slug: str(meta.name),
      description: str(meta.description) || undefined,
      folder: str(meta.folder) || undefined,
      dataset: str(spec.dataset),
      blocks: asList(shown.layout).map(readBlock),
      output: { name: str(shown.name), renderer: str(shown.renderer), page: shown.page },
    }
  }, report)
}

function readBlock(v: Yaml): ReportBlockInput {
  const b = asMap(v)
  const kind = str(b.kind)
  if (kind === 'stat') {
    const value = asMap(b.value)
    return {
      kind, title: str(b.label) || str(b.title), dataset: str(b.dataset) || undefined,
      field: str(value.field), aggregate: str(value.aggregate) || undefined,
    }
  }
  if (kind === 'table') {
    return {
      kind, title: str(b.title), dataset: str(b.dataset) || undefined,
      columns: asList(b.columns).map(str), pageSize: num(b.pageSize),
    }
  }
  if (kind === 'chart') {
    const x = asMap(b.x)
    const y = asMap(b.y)
    // The builder's palette has one entry per chart type, so a chart comes
    // back as the entry that would have drawn it rather than as `chart`.
    return {
      kind: str(b.chart) || 'bar', chart: str(b.chart) || 'bar',
      title: str(b.title), dataset: str(b.dataset) || undefined,
      groupBy: str(x.field), grain: str(x.grain) || undefined,
      field: str(y.field), aggregate: str(y.aggregate) || undefined,
    }
  }
  return { kind, title: str(b.title) || str(b.text), dataset: str(b.dataset) || undefined }
}

/** A schedule. */
export function readSchedule(text: string): Loaded<ScheduleInput> {
  return load(text, (meta, spec) => {
    const burst = asMap(spec.burst)
    const deliver = asMap(asList(spec.deliver)[0])
    const onFailure = asMap(spec.onFailure)
    const bind = Object.values(asMap(burst.bind)).map(str)
    return {
      name: str(meta.title) || str(meta.name),
      slug: str(meta.name),
      report: str(spec.report),
      output: str(spec.output),
      cron: str(spec.cron),
      timezone: str(spec.timezone),
      burstDataset: str(asMap(burst.over).dataset) || undefined,
      recipientField: bindField(bind[0]),
      to: str(deliver.to),
      subject: str(deliver.subject) || undefined,
      channel: str(deliver.via) || undefined,
      filename: str(asMap(deliver.attach).filename) || undefined,
      concurrency: num(burst.concurrency),
      retries: num(onFailure.retries),
      alert: str(onFailure.alert) || undefined,
    }
  }, schedule)
}

/** The column name inside `{{ .row.x }}`, which is what the form asks for. */
function bindField(expr: string | undefined): string | undefined {
  const m = /\{\{\s*\.row\.([A-Za-z0-9_]+)\s*\}\}/.exec(expr ?? '')
  return m ? m[1] : undefined
}
