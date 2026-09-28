import { carryOver, document, fromYaml, toYaml, unmodelled, type Yaml } from './yaml'
import { reference, referenced, sourceSecret } from './secrets'
import type { BasemapProvider, Field, Param, TileMap } from './types'

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
  /**
   * How the object store is reached, for the sources that are not a database.
   *
   * `credentials` is a `${secret:name}` reference like every other credential
   * in this format — the pairs behind it live in the secret, not in the file.
   * Held here rather than in `dsn` because an object store is addressed rather
   * than connected to, and packing four fields into one string would make them
   * unreadable in the one place an operator looks.
   */
  region?: string
  storeEndpoint?: string
  credentials?: string
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
  /**
   * The secret the connection string reads its password from.
   *
   * Read out of a stored one, so that a connection string rebuilt from its
   * parts names the same secret: a source whose file says
   * `${secret:warehouse-password}` keeps reading that when its host changes,
   * rather than switching to a name nothing has stored. Absent, the secret is
   * named after the source — see passwordSecret.
   */
  passwordSecret?: string
  /**
   * Whether the stored string reads that secret `|url` — encoded for the URL
   * it sits in. Kept, so a string rebuilt from new parts reads the stored value
   * the way the old one did: a password somebody encoded by hand, to survive a
   * URL before the server could do it, would otherwise be encoded twice.
   */
  passwordInUrl?: boolean
  /**
   * The stored connection string's options — `?sslmode=require` — read back so
   * that a string rebuilt from new parts keeps them. A move to a new host is
   * not a decision to stop encrypting the connection.
   */
  dsnOptions?: string
  /* The limits, kept as read. The form does not ask for them, and defaulting
     an existing source back to a million rows would raise a ceiling somebody
     lowered deliberately. */
  maxRows?: number
  statementTimeout?: string
}

/**
 * Where a source's password is stored: the secret its connection string
 * already reads, or `<name>_password` for one that reads none yet.
 *
 * One function for the two halves that have to agree — the reference written
 * into the definition and the name the wizard stores the typed password under.
 * They were one line apart in intent and never connected: the file named a
 * secret, the form threw the password away, and every database connected
 * through the portal named a password that nothing held.
 */
export function passwordSecret(input: Pick<SourceInput, 'slug' | 'passwordSecret'>): string {
  return input.passwordSecret ?? sourceSecret(input.slug, 'password')
}

/**
 * The connection string a save writes: the one typed, the one stored, or —
 * undefined — one built from the parts.
 *
 * Typed where the form shows one: a SQLite path, or a driver this build has no
 * screen for. Otherwise the stored one while nothing about where it connects
 * has changed, because not every DSN decomposes into host and database and
 * user, and rebuilding one that was only ever displayed would replace a
 * working connection with a guess.
 *
 * A new password changes nothing about the string when it already names a
 * secret: the password is stored under that name and the string goes on
 * reading it. It is rebuilt when it names none, because then it has to be
 * made to.
 *
 * This decided `form.dsn || …` inside the form, and the form seeds that field
 * with the stored string for every kind of source — so for a database the
 * stored string always won, and a source moved to a new host went on
 * connecting to the old one.
 */
export function connectionFor(
  shape: string,
  form: { dsn: string; host: string; port: number; database: string; user: string; password: string },
  stored?: SourceInput,
): string | undefined {
  if (shape === 'dsn') return form.dsn || undefined
  if (!stored?.dsn) return undefined
  const untouched = form.host === (stored.host ?? '') && form.port === (stored.port ?? 5432)
    && form.database === (stored.database ?? '') && form.user === (stored.user ?? '')
  return untouched && (form.password === '' || !!stored.passwordSecret) ? stored.dsn : undefined
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
    // Only when set. An empty region is not the same as no region — one is a
    // field the store will be asked to honour, and writing it out blank puts a
    // key in the file that means nothing and reads as if it did.
    if (input.region) spec.region = input.region
    if (input.storeEndpoint) spec.endpoint = input.storeEndpoint
    if (input.credentials) spec.credentials = input.credentials
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
 * A connection string with the password left as a reference, in the form the
 * driver reads.
 *
 * Written out rather than assembled from the parts at read time, because the
 * format takes one DSN and an operator reading the file should see the shape
 * of what will be connected to.
 *
 * Each in its driver's own form, because they are three different grammars and
 * this wrote one URL for all of them. MySQL's driver reads
 * `user:password@tcp(host:port)/db` and nothing else — a URL is a parse error —
 * and takes the password as written. SQL Server's reads a URL but takes its path
 * for the instance name, so the database is `?database=`. Postgres is the URL it
 * always was. Where the password sits in a URL it is referenced `|url`, and the
 * server encodes it there.
 */
function dsn(driver: string, input: SourceInput): string {
  const user = input.user || 'cronos'
  const host = input.host || 'localhost'
  const secret = passwordSecret(input)
  const db = input.database ?? ''
  // New strings encode; a stored one keeps the form it had — see passwordInUrl.
  const inUrl = input.passwordSecret ? input.passwordInUrl === true : true
  switch (driver) {
    case 'mysql':
      return `${user}:${reference(secret)}@tcp(${host}:${input.port || 3306})/${db}`
        + (input.dsnOptions ?? '?parseTime=true')
    case 'sqlserver':
      return `sqlserver://${user}:${reference(secret, inUrl)}@${host}${input.port ? `:${input.port}` : ''}`
        + withDatabase(input.dsnOptions, db)
    default:
      return `${driver}://${user}:${reference(secret, inUrl)}@${host}${input.port ? `:${input.port}` : ''}`
        + `/${db}${input.dsnOptions ?? ''}`
  }
}

/** A SQL Server query string with the database first, keeping whatever else
 *  the stored one said — `encrypt=disable` is somebody's decision. */
function withDatabase(options: string | undefined, database: string): string {
  const rest = new URLSearchParams((options ?? '').replace(/^\?/, ''))
  rest.delete('database')
  const q = new URLSearchParams(database ? { database } : {})
  for (const [k, v] of rest) q.append(k, v)
  const out = q.toString()
  return out ? `?${out}` : ''
}

export interface DatasetInput {
  name: string
  slug: string
  description?: string
  source: string
  query: string
  fields: Field[]
  /**
   * The questions this dataset accepts.
   *
   * Carried through an edit before this was modelled, so a parameterised
   * dataset survived being opened and saved — but could only be created or
   * changed by editing the file, which made the portal a second-class way to
   * author exactly the datasets that need the most care.
   */
  params?: Param[]
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
    // Before fields, because that is the order the format's own examples use
    // and a file somebody reads afterwards should look like the ones they have
    // read before.
    params: input.params && input.params.length > 0
      ? input.params.map(param)
      : undefined,
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

function param(p: Param): Yaml {
  return {
    name: p.name,
    type: p.type,
    label: p.label || undefined,
    required: p.required || undefined,
    multiple: p.multiple || undefined,
    // Enum only. Values on anything else is a constraint the engine does not
    // apply, which reads as one that does.
    values: p.type === 'enum' && p.values?.length ? p.values : undefined,
    default: p.default || undefined,
  }
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

/**
 * One control on a report's filter bar.
 *
 * `bind` is a field per dataset, and it is explicit because a report's blocks
 * may read different datasets and guessing is how a filter silently applies to
 * half a screen. A dataset with no entry is unaffected, which is a legitimate
 * outcome the viewer is required to show on the block.
 */
export interface ReportFilterInput {
  name: string
  label?: string
  /** string, number, bool, date, enum or area. */
  type: string
  /** The permitted values. Enum only, and required there. */
  values?: string[]
  /** Dataset name to the field this filter narrows in it — for an area, the
   *  pair of fields, `lat,lon`. */
  bind: Record<string, string>
  /** The control the filter bar draws. Undefined is the type's default. */
  control?: string
}

export interface ReportInput {
  name: string
  slug: string
  description?: string
  folder?: string
  dataset: string
  /**
   * The report's shared filters.
   *
   * These were neither written nor read for as long as the builder existed,
   * and `drops` did not cover them either — so opening a report that had
   * filters and saving it deleted every one of them with nothing on screen
   * saying so. They are the report's entire interactive surface.
   */
  filters?: ReportFilterInput[]
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
  metrics?: {
    field: string
    aggregate?: string
    label?: string
    draw?: string
    secondary?: boolean
  }[]
  target?: { field?: string; aggregate?: string; value?: number; label?: string }
  series?: string
  stacked?: boolean
  /** A plot's horizontal measure. */
  xField?: string
  /** A bubble's radius measure. */
  sizeField?: string
  /**
   * The builder's own type rather than a copy of it. The copy lagged: it had
   * no `region` or `simplify`, so opening a map that set either and saving it
   * dropped them — named in the drop warning, and dropped all the same.
   */
  map?: TileMap
  columns?: string[]
  pageSize?: number
  /** Narrows this block alone, as SQL the server compiles. */
  filter?: string
  sort?: { field: string; dir?: string }[]
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
    filters: reportFilters(input.filters),
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
/**
 * The palette entries that are charts, and which chart each one is.
 *
 * Identity today, and kept as a map rather than collapsed to a Set because the
 * two lists are allowed to diverge: a palette entry is a thing to drop on a
 * canvas and a chart type is a thing a renderer draws, and the moment one
 * gains an entry the other should not, this is where that lives.
 */
const CHARTS: Record<string, string> = {
  bar: 'bar', line: 'line', area: 'area', pie: 'pie', donut: 'donut',
  scatter: 'scatter', bubble: 'bubble', map: 'map',
  combo: 'combo', funnel: 'funnel', waterfall: 'waterfall',
  heatmap: 'heatmap', gauge: 'gauge', treemap: 'treemap',
}

/** The chart types whose horizontal axis is a measure rather than a bucket. */
const PLOTTED = new Set(['scatter', 'bubble'])

/** The chart types that read a list of measures rather than one. */
const METERED = new Set(['combo', 'funnel'])

/**
 * The filter bar, or nothing when the report has none.
 *
 * A half-finished filter — named but not yet bound to a field — is written out
 * rather than quietly dropped. The server refuses it with a sentence naming
 * what is missing, which the author can act on; deleting their work on save
 * and saying nothing is the alternative, and it is worse.
 */
function reportFilters(filters: ReportInput['filters']): Yaml {
  if (!filters || filters.length === 0) return undefined
  return filters
    .filter((f) => f.name)
    .map((f) => ({
      name: f.name,
      label: f.label || undefined,
      type: f.type,
      // Enum only. Writing them on a date filter would store a list the
      // server refuses, which is a save that fails for a reason the form did
      // not show.
      values: f.type === 'enum' && f.values?.length ? f.values : undefined,
      bind: Object.fromEntries(
        Object.entries(f.bind ?? {}).filter(([reads, narrows]) => reads && narrows)),
      control: f.control || undefined,
    }))
}

function block(b: ReportBlockInput): Yaml {
  const reads = b.dataset || undefined
  // Every kind takes them, so they are folded in once rather than repeated in
  // each branch — and a kind that gains support for one later gets it here.
  const narrowed = (v: Record<string, Yaml>): Yaml => ({
    ...v,
    filter: b.filter?.trim() || undefined,
    sort: b.sort && b.sort.length > 0
      ? b.sort.map((k) => ({ field: k.field, dir: k.dir || undefined }))
      : undefined,
  })

  if (b.kind === 'stat') {
    return narrowed({
      kind: 'stat', dataset: reads, label: b.title,
      value: { field: b.field, aggregate: b.aggregate ?? 'sum' },
    })
  }
  if (b.kind === 'table') {
    return narrowed({
      kind: 'table', dataset: reads, title: b.title,
      columns: b.columns ?? [], pageSize: b.pageSize || undefined,
    })
  }
  if (CHARTS[b.kind] || b.kind === 'chart') {
    const chart = CHARTS[b.kind] ?? b.chart ?? 'bar'
    return narrowed({
      kind: 'chart', dataset: reads, title: b.title, chart,
      // A plot's x names what each dot *is* and its xValue is where the dot
      // sits. One field cannot be both a dimension and a measure, and letting
      // it try is how a date grain ends up on a number.
      x: chart === 'funnel' && (b.metrics?.length ?? 0) > 0
        ? undefined
        : { field: b.groupBy, grain: PLOTTED.has(chart) ? undefined : b.grain || undefined },
      xValue: PLOTTED.has(chart) && b.xField
        ? { field: b.xField, aggregate: b.aggregate ?? 'sum' }
        : undefined,
      // A metered chart's measures replace y rather than joining it — both
      // would be two answers to what the chart measures.
      y: METERED.has(chart) ? undefined : { field: b.field, aggregate: b.aggregate ?? 'sum' },
      metrics: METERED.has(chart) ? metrics(b.metrics) : undefined,
      target: chart === 'gauge' ? targetOf(b.target) : undefined,
      series: b.series ? { field: b.series } : undefined,
      stacked: b.stacked || undefined,
      size: chart === 'bubble' && b.sizeField
        ? { field: b.sizeField, aggregate: b.aggregate ?? 'sum' }
        : undefined,
      map: chart === 'map' ? mapSpec(b.map) : undefined,
    })
  }
  return narrowed({ kind: b.kind, dataset: reads, title: b.title, text: b.title })
}

function metrics(list: ReportBlockInput['metrics']): Yaml {
  if (!list || list.length === 0) return undefined
  return list
    .filter((m) => m.field)
    .map((m) => ({
      field: m.field,
      aggregate: m.aggregate ?? 'sum',
      label: m.label || undefined,
      draw: m.draw || undefined,
      axis: m.secondary ? 'secondary' : undefined,
    }))
}

/**
 * A gauge's target: a column or a number, never both.
 *
 * The server refuses both, because honouring one silently makes the other look
 * honoured — so the builder writes whichever the author chose rather than
 * whatever is left in the form from the other.
 */
function targetOf(t: ReportBlockInput['target']): Yaml {
  if (!t) return undefined
  if (t.value !== undefined) {
    return { value: t.value, label: t.label || undefined }
  }
  if (!t.field) return undefined
  return { field: t.field, aggregate: t.aggregate ?? 'sum', label: t.label || undefined }
}

/**
 * A map's geography.
 *
 * Nested under `map:` in the format rather than flattened into the block like
 * every other kind, because a map adds a dozen fields and the flat union is
 * what keeps the other four readable.
 */
function mapSpec(m: ReportBlockInput['map']): Yaml {
  if (!m) return undefined
  return {
    layers: m.layers && m.layers.length > 0 ? m.layers : undefined,
    geometry: m.geometry || undefined,
    region: m.region || undefined,
    lat: m.lat || undefined,
    lon: m.lon || undefined,
    toLat: m.toLat || undefined,
    toLon: m.toLon || undefined,
    // Zero is the server's default for both — hexagons sized to the data,
    // the usual simplification — which is also what absence says. A negative
    // simplify is not zero: it keeps every vertex, and `||` leaves it alone.
    hexKm: m.hexKm || undefined,
    simplify: m.simplify || undefined,
    classify: m.classify || undefined,
    // Only with custom classes, which are the only ones that read them: the
    // server refuses breaks beside any other method rather than ignore them.
    breaks: m.classify === 'custom' && m.breaks?.length ? m.breaks : undefined,
    ramp: m.ramp || undefined,
    // Zero is a midpoint, and the default one: written only when it is not.
    midpoint: m.ramp === 'diverging' && m.midpoint ? m.midpoint : undefined,
    basemap: basemapOf(m),
    overlays: m.overlays?.length ? m.overlays : undefined,
  }
}

/**
 * The basemap, in whichever of its two forms the author chose.
 *
 * A provider wins when both are somehow set, and the url's half is not
 * written: the server refuses a basemap that names both, and one that brings
 * a provider's tiles and somebody else's credit line besides.
 */
function basemapOf(m: TileMap): Yaml {
  if (m.provider) {
    return {
      provider: m.provider,
      style: m.style || undefined,
      key: m.key || undefined,
      language: m.language || undefined,
      region: m.basemapRegion || undefined,
      maxZoom: m.maxZoom || undefined,
    }
  }
  // Written only as a pair. A tile template with no credit line is a report
  // that puts our customer in breach of the tile source's terms, so the
  // server refuses one — and emitting half of it here would turn that into a
  // save that fails with nothing on screen having asked for it.
  if (m.basemap) {
    return { url: m.basemap, attribution: m.attribution || undefined, maxZoom: m.maxZoom || undefined }
  }
  return undefined
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
   * The version this was read at, so a save can say what it started from.
   *
   * Travels with the document rather than beside it, for the same reason
   * `drops` does: a save needs both, and a caller holding one without the
   * other has something it cannot use.
   *
   * Empty where nothing was loaded from a server — sample mode, or a form
   * creating something new — which is also what says "store this
   * unconditionally".
   */
  version?: string
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
/*
KINDS maps a stored driver back to the card the picker shows.

Only the names that differ need an entry — everything else falls through as
itself, which is why `sqlserver` works without one. What that fallthrough cannot
do is invent a card for a driver the picker has none for: `sqlite` landed as the
kind `sqlite`, matched nothing, and the connect step rendered blank with three
ticks above it and Continue greyed out. See DataSourceForm.
*/
const KINDS: Record<string, string> = {
  postgres: 'postgres', mysql: 'mysql', 'object-store': 'objectstore',
  // Both spellings of the same product, as the engine accepts.
  mssql: 'sqlserver',
}

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
      region: spec.region ? str(spec.region) : undefined,
      storeEndpoint: spec.endpoint ? str(spec.endpoint) : undefined,
      // Read back, unlike a password: what the file holds is the reference and
      // not the credential, so showing it costs nothing and leaving it blank
      // would drop it on the next save.
      credentials: spec.credentials ? str(spec.credentials) : undefined,
      // Before the parsed parts, which never include a dsn and so cannot
      // overwrite it.
      dsn: connection || undefined,
      ...passwordReference(connection),
      maxRows: num(limits.maxRows),
      statementTimeout: str(limits.statementTimeout) || undefined,
      ...parseDsn(connection),
    }
  }, dataSource)
}

/**
 * The secret a connection string's password names, when it names one.
 *
 * `postgres://reader:${secret:warehouse-password}@db/analytics` answers
 * `warehouse-password`. A literal password, or none, answers nothing — and
 * then a password typed on edit is stored under the source's own name and the
 * string is rebuilt to read it, because keeping one that reads no secret
 * would store a password nothing opens.
 */
function passwordReference(connection: string): Pick<SourceInput, 'passwordSecret' | 'passwordInUrl'> {
  const m = /^(?:[a-z][a-z0-9+.-]*:\/\/)?[^:@/]*:(.*)@(?:tcp\(|[^@]*$)/i.exec(connection)
  const name = m ? referenced(m[1]) : undefined
  if (!name) return {}
  return { passwordSecret: name, passwordInUrl: /\|url\}\s*$/.test(m?.[1] ?? '') }
}

/**
 * Pulls the pieces back out of a connection string, password included or not:
 * a URL, or MySQL's own `user:password@tcp(host:port)/db`. SQL Server's
 * database is read from `?database=`, where its driver reads it, and the rest
 * of the query kept.
 */
function parseDsn(connection: string): Partial<SourceInput> {
  const mysql = /^([^:@/]*)(?::.*)?@tcp\(([^:)]*)(?::(\d+))?\)\/([^?]*)(\?.*)?$/.exec(connection)
  if (mysql) {
    return {
      user: mysql[1] || undefined, host: mysql[2] || undefined,
      port: mysql[3] ? Number(mysql[3]) : undefined,
      database: mysql[4] || undefined, dsnOptions: mysql[5] || undefined,
    }
  }
  const m = /^([a-z0-9-]+):\/\/(?:([^:@/]*)(?::.*)?@)?([^:/?@]*)(?::(\d+))?(?:\/([^?]*))?(\?.*)?$/.exec(connection)
  if (!m) return {}
  const out: Partial<SourceInput> = {
    user: m[2] || undefined,
    host: m[3] || undefined,
    port: m[4] ? Number(m[4]) : undefined,
    database: m[5] || undefined,
    dsnOptions: m[6] || undefined,
  }
  if (m[1] === 'sqlserver') {
    const q = new URLSearchParams((m[6] ?? '').replace(/^\?/, ''))
    out.database = q.get('database') ?? out.database
    q.delete('database')
    out.dsnOptions = q.toString() ? `?${q.toString()}` : undefined
  }
  return out
}

/** A dataset. */
export function readDataset(text: string): Loaded<DatasetInput> {
  return load(text, (meta, spec) => ({
    name: str(meta.title) || str(meta.name),
    slug: str(meta.name),
    description: str(meta.description) || undefined,
    source: str(asMap(asList(spec.sources)[0]).ref),
    query: str(spec.query),
    // Undefined rather than an empty list, so a dataset that declares none
    // reads back as one that declares none — and an edit does not introduce a
    // `params: []` nobody wrote.
    params: spec.params ? asList(spec.params).map(readParam) : undefined,
    fields: asList(spec.fields).map(readField),
    predicate: str(asMap(asList(spec.rowLevelSecurity)[0]).predicate) || undefined,
  }), dataset)
}

function readParam(v: Yaml): Param {
  const p = asMap(v)
  return {
    name: str(p.name),
    type: (str(p.type) || 'string') as Param['type'],
    label: str(p.label) || undefined,
    required: p.required === true,
    multiple: p.multiple === true,
    values: asList(p.values).map(str),
    // Read as written. A date default is a date in the file and a string here,
    // because the form edits text and the emitter quotes what needs quoting.
    default: p.default == null ? undefined : String(p.default),
  }
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
      filters: asList(spec.filters).map(readFilter),
      blocks: asList(shown.layout).map(readBlock),
      output: { name: str(shown.name), renderer: str(shown.renderer), page: shown.page },
    }
  }, report)
}

function readFilter(v: Yaml): ReportFilterInput {
  const f = asMap(v)
  return {
    name: str(f.name),
    label: str(f.label) || undefined,
    type: str(f.type) || 'string',
    values: asList(f.values).map(str),
    bind: Object.fromEntries(
      Object.entries(asMap(f.bind)).map(([reads, narrows]) => [reads, str(narrows)])),
    control: str(f.control) || undefined,
  }
}

function readBlock(v: Yaml): ReportBlockInput {
  const b = asMap(v)
  const kind = str(b.kind)
  const narrowing = {
    filter: str(b.filter) || undefined,
    sort: asList(b.sort).map((k) => {
      const key = asMap(k)
      return { field: str(key.field), dir: str(key.dir) || undefined }
    }),
  }
  if (kind === 'stat') {
    const value = asMap(b.value)
    return {
      ...narrowing,
      kind, title: str(b.label) || str(b.title), dataset: str(b.dataset) || undefined,
      field: str(value.field), aggregate: str(value.aggregate) || undefined,
    }
  }
  if (kind === 'table') {
    return {
      ...narrowing,
      kind, title: str(b.title), dataset: str(b.dataset) || undefined,
      columns: asList(b.columns).map(str), pageSize: num(b.pageSize),
    }
  }
  if (kind === 'chart') {
    const x = asMap(b.x)
    const y = asMap(b.y)
    const chart = str(b.chart) || 'bar'
    // The builder's palette has one entry per chart type, so a chart comes
    // back as the entry that would have drawn it rather than as `chart`.
    return {
      ...narrowing,
      kind: chart, chart,
      title: str(b.title), dataset: str(b.dataset) || undefined,
      groupBy: str(x.field), grain: str(x.grain) || undefined,
      field: str(y.field), aggregate: str(y.aggregate) || undefined,
      series: str(asMap(b.series).field) || undefined,
      stacked: b.stacked === true || undefined,
      xField: str(asMap(b.xValue).field) || undefined,
      sizeField: str(asMap(b.size).field) || undefined,
      metrics: asList(b.metrics).map((raw) => {
        const metric = asMap(raw)
        return {
          field: str(metric.field),
          aggregate: str(metric.aggregate) || undefined,
          label: str(metric.label) || undefined,
          draw: str(metric.draw) || undefined,
          secondary: str(metric.axis) === 'secondary' || undefined,
        }
      }),
      target: chart === 'gauge' ? readTarget(asMap(b.target)) : undefined,
      map: chart === 'map' ? readMap(asMap(b.map)) : undefined,
    }
  }
  return { ...narrowing, kind, title: str(b.title) || str(b.text), dataset: str(b.dataset) || undefined }
}

/**
 * A map's geography, back from the file.
 *
 * Every key, including the ones the inspector never shows. A map lives inside
 * `outputs[].layout[]`, which the builder rewrites wholesale, so carry-over
 * cannot reach it: whatever is not read here is gone on the next save. That
 * is how `region` and `simplify` were lost, and how a basemap's key would have
 * been — moving a report onto whichever account the deployment bills.
 */
function readMap(m: Doc): TileMap {
  const basemap = asMap(m.basemap)
  return {
    layers: asList(m.layers).map(str),
    geometry: str(m.geometry) || undefined,
    region: str(m.region) || undefined,
    lat: str(m.lat) || undefined,
    lon: str(m.lon) || undefined,
    toLat: str(m.toLat) || undefined,
    toLon: str(m.toLon) || undefined,
    hexKm: num(m.hexKm),
    simplify: num(m.simplify),
    classify: str(m.classify) || undefined,
    breaks: asList(m.breaks).length ? asList(m.breaks).map(Number) : undefined,
    ramp: str(m.ramp) || undefined,
    midpoint: num(m.midpoint),
    // Cast rather than checked. The server refuses a provider it has no code
    // for, so a stored one is one of these; a check here would be a second
    // list to keep in step with that one.
    provider: (str(basemap.provider) || undefined) as BasemapProvider | undefined,
    style: str(basemap.style) || undefined,
    key: str(basemap.key) || undefined,
    language: str(basemap.language) || undefined,
    basemapRegion: str(basemap.region) || undefined,
    maxZoom: num(basemap.maxZoom),
    basemap: str(basemap.url) || undefined,
    attribution: str(basemap.attribution) || undefined,
    // Whole, as the file has them: each is a block the server checks, and
    // nothing here edits one.
    overlays: asList(m.overlays).length ? asList(m.overlays) : undefined,
  }
}

/** A gauge's target, back from the file. */
function readTarget(t: Record<string, Yaml>): ReportBlockInput['target'] {
  const value = num(t.value)
  if (value !== undefined) {
    return { value, label: str(t.label) || undefined }
  }
  if (!str(t.field)) return undefined
  return {
    field: str(t.field),
    aggregate: str(t.aggregate) || undefined,
    label: str(t.label) || undefined,
  }
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
