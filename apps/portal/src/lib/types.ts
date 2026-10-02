import type { Yaml } from './yaml'

/** Mirrors the definition format in docs/report-format.md. */

export type FieldType = 'string' | 'number' | 'decimal' | 'date' | 'bool' | 'enum'
export type FieldRole = 'dimension' | 'measure'

export interface Field {
  name: string
  /** What a person calls it. The UI never shows `name` when this exists. */
  label: string
  type: FieldType
  role: FieldRole
  /** enum only */
  values?: string[]
  /**
   * How to render the value.
   *
   * `preformatted` means the engine already did it — the one that knew the
   * currency, the locale and the rounding rule — so nothing downstream should
   * touch it. Without it a value arriving as "19,800" meets Number("19,800")
   * and the column reads NaN.
   */
  format?: 'currency' | 'percent' | 'number' | 'preformatted'
  hidden?: boolean
}

export interface Dataset {
  name: string
  label: string
  description?: string
  fields: Field[]
}

/* -- Filters ------------------------------------------------------------- */

export type Operator =
  | 'is' | 'isNot' | 'contains' | 'startsWith' | 'isEmpty' | 'isNotEmpty'
  | 'anyOf' | 'noneOf'
  | 'eq' | 'gt' | 'gte' | 'lt' | 'lte' | 'between'
  | 'inLast' | 'inNext' | 'onOrAfter' | 'onOrBefore' | 'thisMonth'
  | 'lastMonth' | 'thisQuarter' | 'lastQuarter' | 'yearToDate'

export interface Condition {
  id: string
  kind: 'condition'
  field: string
  op: Operator
  /** Shape depends on the operator: scalar, [lo,hi], string[] or {n,unit}. */
  value?: unknown
}

export interface Group {
  id: string
  kind: 'group'
  /** Only ever a conjunction with RLS above it — see docs/report-format.md. */
  join: 'and' | 'or'
  children: FilterNode[]
}

export type FilterNode = Condition | Group

/* -- Reports ------------------------------------------------------------- */

/**
 * What the palette calls a block.
 *
 * Not the same list as the report format's block kinds, and deliberately so:
 * the format splits a chart into `kind: chart` plus a chart type, because a
 * renderer that learns a new block kind learns a new concept and one that
 * learns a new chart type learns a case. A palette is a list of things to drop
 * on a canvas, which is a different question — definitions.ts translates.
 */
export type TileKind =
  | 'stat' | 'table'
  | 'bar' | 'column' | 'line' | 'area' | 'pie' | 'donut'
  | 'scatter' | 'bubble' | 'map'
  | 'combo' | 'funnel' | 'waterfall' | 'heatmap' | 'gauge' | 'treemap'
  | 'radar' | 'bullet' | 'histogram' | 'boxplot' | 'sankey' | 'sunburst' | 'calendar'

/** The tiles that bucket a dimension and fold a measure. */
export const CATEGORICAL: TileKind[] = [
  'bar', 'column', 'line', 'area', 'pie', 'donut', 'waterfall', 'heatmap', 'treemap', 'radar',
  'sankey', 'sunburst', 'calendar',
]

/** The tiles that read a list of measures rather than one. */
export const METERED: TileKind[] = ['combo', 'funnel']

/** The tiles that need a second dimension to place a value. */
export const GRIDDED: TileKind[] = ['heatmap']

/** The tiles whose series is a second dimension rather than an optional
 *  split: a heatmap's other axis, where a sankey's flows go, and the ring a
 *  sunburst's parts sit in. */
export const PAIRED: TileKind[] = ['heatmap', 'sankey', 'sunburst']

/** The tiles that fold the whole set to one number. */
export const FOLDED: TileKind[] = ['gauge']

/** The tiles read against a target: a gauge's one number, or each bullet. */
export const TARGETED: TileKind[] = ['gauge', 'bullet']

/** The tiles that read a number row by row: a histogram's bins, a box plot's
 *  quartiles. Neither folds its measure with an aggregate. */
export const SPREAD: TileKind[] = ['histogram', 'boxplot']

/** The tiles that read two measures against each other. */
export const PLOTS: TileKind[] = ['scatter', 'bubble']

/** The tiles that can draw more than one series at once. */
export const MULTI_SERIES: TileKind[] = [
  'bar', 'column', 'line', 'area', 'scatter', 'bubble', 'heatmap', 'treemap', 'radar',
  'sankey', 'sunburst',
]

/** The tiles a series dimension stacks on rather than drawing beside. */
export const STACKABLE: TileKind[] = ['bar', 'column', 'area']

export interface Tile {
  id: string
  kind: TileKind
  title: string
  /** Grid span out of 12. */
  span: number
  /**
   * Overrides the report's dataset for this block alone. Undefined means the
   * report default. This is what lets one report combine invoices and
   * shipments — the job a separate Dashboard kind would have existed to do.
   */
  dataset?: string
  field?: string
  groupBy?: string
  /** What a date grouping is bucketed into: a month, a week. Undefined is
   *  each date as it stands. */
  grain?: string
  series?: string
  aggregate?: 'sum' | 'count' | 'avg' | 'min' | 'max'
  /** Draws a multi-series bar, column or area as one stack per bucket —
   *  `percent` for each part as its share of its bucket's whole. */
  stacked?: boolean | 'percent'
  /** The horizontal measure of a scatter or bubble, whose x is a number
   *  rather than a bucket. */
  xField?: string
  /** The measure a bubble's radius reads. */
  sizeField?: string
  /** The geography a map block reads. Undefined on every other kind. */
  map?: TileMap
  /** Several measures, for the kinds that read a list — a combo's bars and
   *  lines, or a funnel whose stages are separate columns. */
  metrics?: TileMetric[]
  /** What a gauge or a bullet chart reads its values against. */
  target?: TileTarget
  /** Where a bullet chart's track changes shade, as fractions of each row's
   *  target. Kept as read; the inspector does not edit them. */
  bands?: number[]
  /** About how many bins a histogram cuts its range into. */
  bins?: number
  /** A stat's number over time, drawn under it. */
  trend?: TileTrend
  columns?: string[]
  /**
   * Narrows this block alone — "of which, overdue".
   *
   * A predicate as the author wrote it, not a built expression. The format
   * takes SQL here and the server compiles it against the dataset, so a
   * builder that offered a field-operator-value row would refuse every
   * predicate that is not one comparison — which is most of the interesting
   * ones.
   */
  filter?: string
  /** A table's ordering, in the order the keys are applied. */
  sort?: { field: string; dir?: 'asc' | 'desc' }[]
}

/**
 * One control on a report's filter bar.
 *
 * `bind` is a field per dataset, and explicit, because a report's blocks may
 * read different datasets — guessing is how a filter silently applies to half
 * a screen. A dataset with no entry is unaffected, which is a legitimate
 * outcome the viewer shows on the block rather than letting it be discovered.
 */
export interface ReportFilter {
  name: string
  label?: string
  /** string, number, bool, date, enum or area. */
  type: string
  /** The permitted values. Enum only. */
  values?: string[]
  /**
   * An area's is a pair — `lat,lon` — because a place is two columns, and the
   * filter narrows by both.
   */
  bind: Record<string, string>
  /** How the filter bar shows it — checkboxes rather than a dropdown, say.
   *  Undefined is the type's default. Kept as read, and let go of when the
   *  type changes, because a control suits some types and not others. */
  control?: string
}

/** One measure of a tile that draws several, and how it is drawn. */
export interface TileMetric {
  field: string
  aggregate?: 'sum' | 'count' | 'avg' | 'min' | 'max'
  label?: string
  /** bar or line, on a combo. */
  draw?: 'bar' | 'line'
  /** Set to read against its own scale. Per measure and never a default: two
   *  scales on one plot is the most-flagged mistake in charting, so it is
   *  reachable and never accidental. */
  secondary?: boolean
}

/** A stat's number per period of a date, and which way is good news. */
export interface TileTrend {
  field: string
  grain?: string
  better?: 'higher' | 'lower'
}

/** What a gauge or a bullet chart measures against: a column, or a number. */
export interface TileTarget {
  field?: string
  aggregate?: 'sum' | 'count' | 'avg' | 'min' | 'max'
  value?: number
  label?: string
}

/**
 * A tile source whose terms the server knows: its URL, the credit line and
 * logo its licence requires, and where its key comes from. A closed set on the
 * server, so adding one is code there rather than a string an author invents.
 */
export type BasemapProvider = 'openstreetmap' | 'mapbox' | 'google'

/**
 * The geography a map tile reads, and what it is drawn over.
 *
 * Flat, like Tile, although the file nests the basemap under `map.basemap`:
 * the form edits one field at a time, and a nested object would make every
 * control spread two levels to change one value. definitions.ts does the
 * nesting, in both directions.
 */
export interface TileMap {
  /** Drawn bottom to top. Empty lets the server infer one from the fields. */
  layers?: string[]
  /** The field carrying GeoJSON, read by the polygon and the line layer alike. */
  geometry?: string
  /**
   * Names each row in place of the block's x — group by a country code, label
   * with the country's name. The builder offers no control for it; it is read
   * and written so that opening a map and saving it does not relabel it.
   */
  region?: string
  lat?: string
  lon?: string
  toLat?: string
  toLon?: string
  /** How far shapes are simplified, in Web Mercator world units. Kept as read:
   *  it is tuned against a payload size, which is not a question for a form. */
  simplify?: number
  /** A hexbin layer's hexagon width, in kilometres. Undefined, or zero, sizes
   *  them to the data. */
  hexKm?: number
  /** A named tile source. Exclusive with `basemap`, because a provider brings
   *  its own tiles and its own credit line. */
  provider?: BasemapProvider
  /** One of the provider's looks. Undefined is its default. */
  style?: string
  /**
   * A `${secret:name}` reference to the provider's key, for a report that
   * bills a different account from the deployment's. The reference is never
   * edited in a form — a key typed into one would land in the file — but kept
   * as read, because dropping it moves the bill. The value behind it is set
   * from the builder and stored as that secret; see BasemapKeys.
   */
  key?: string
  /** Google only: the language its labels are drawn in. Kept as read. */
  language?: string
  /**
   * Google only: the region whose conventions its borders and labels follow.
   * `basemap.region` in the file, and named apart here because `region` on
   * this type is already the label field. Kept as read.
   */
  basemapRegion?: string
  /** How far the viewer zooms, for either kind of basemap. Kept as read. */
  maxZoom?: number
  /**
   * An XYZ tile template, for a tile server with no provider name. Undefined
   * draws no basemap, which is the default: a basemap is a request from the
   * reader's browser to a third party. Empty is a template still being typed,
   * and writes nothing either.
   */
  basemap?: string
  attribution?: string
  /**
   * How the shaded layers' values are split into the ramp's six shades:
   * quantile (the default), equal, jenks or custom — the last with `breaks`,
   * the upper bounds of its classes, ascending.
   */
  classify?: string
  breaks?: number[]
  /** sequential (the default) or diverging, which takes two hues either side
   *  of `midpoint` — zero unless set. */
  ramp?: string
  midpoint?: number
  /** How far a radius layer's circles reach, in kilometres on the ground. */
  radiusKm?: number
  /** Names the regions and places on the map itself. */
  labels?: boolean
  /** Moves a flow layer's arcs from where each starts to where it lands. */
  animate?: boolean
  /** A field of H3 cell ids for an h3 layer to shade; without it the layer
   *  bins the places at lat and lon. */
  h3?: string
  /** How fine an h3 layer's cells are, 1 to 15. Unset sizes binned cells to
   *  the data, and draws indexed ones as they are. */
  h3Resolution?: number
  /**
   * A date field the map plays through, a period at a time — `map.time` in
   * the file — and the period: day, week, month, quarter or year. The map
   * opens on every period together. Not beside hexagons or H3 cells, which
   * the server refuses: each is folded from every place at once.
   */
  time?: string
  timeGrain?: string
  /**
   * Other datasets drawn over this map — depots over the deliveries around
   * them — each a block of its own under `map.overlays`. Kept as read: the
   * builder has no control for them yet, and a map rewritten without them
   * would lose a layer nobody asked to remove.
   */
  overlays?: Yaml[]
}

/**
 * One question a dataset accepts.
 *
 * The only caller-supplied input that reaches a query, and it reaches it as a
 * bind argument — there is deliberately no parameter that substitutes SQL
 * text. See docs/report-format.md.
 */
export interface Param {
  name: string
  type: 'string' | 'number' | 'bool' | 'date' | 'enum'
  label?: string
  required?: boolean
  /** Accepts a list, bound as one — for `= ANY(…)` and `IN`. */
  multiple?: boolean
  /** Enum only, and required there: an enum with no values accepts anything. */
  values?: string[]
  default?: string
}

export interface Report {
  name: string
  label: string
  folder: string
  description?: string
  dataset: string
  tiles: Tile[]
  filter: Group
  updatedAt: string
  updatedBy: string
  outputs: ('interactive' | 'pdf' | 'xlsx')[]
  scheduled?: { cron: string; recipients: number }
}

/* -- Secrets -------------------------------------------------------------
 *
 * Not the file format: a definition only ever names a secret, as
 * `${secret:name}`, and these are the server's answer about what stands
 * behind each name. Here because they are the other half of that reference —
 * see lib/secrets.ts for the rules a name follows.
 */

/**
 * Where the value behind a name comes from today.
 *
 * `project` was stored in this project, through the portal or the API.
 * `deployment` is the server's own environment or a mounted file, which the
 * project cannot see into or change. `missing` is nothing at all — and
 * whatever reads it is broken until something answers.
 */
export type SecretSource = 'project' | 'deployment' | 'missing'

/** A definition that names a secret. */
export interface SecretUse {
  kind: 'DataSource' | 'Report'
  name: string
}

/**
 * One secret, as somebody managing them sees it.
 *
 * There is no value, and not because it was left out: no request returns one,
 * to anybody. A secret is set, used and replaced; whoever needs to know what
 * it is has it wherever they copied it from.
 */
export interface SecretEntry {
  name: string
  source: SecretSource
  /** When a stored one was last set. Absent for the other two sources. */
  updatedAt?: string
  /** The account that set it — an id, not a name. */
  updatedBy?: string
  usedBy: SecretUse[]
}

export interface SecretList {
  /**
   * Whether this deployment can keep a secret at all.
   *
   * False on a server with no CRONOS_SECRETS_KEY. The list still answers,
   * because what a project uses and where each value comes from are worth
   * knowing whether or not anything can be stored.
   */
  store: boolean
  /** Sorted by name: every one stored here, and every one a definition names. */
  secrets: SecretEntry[]
}
