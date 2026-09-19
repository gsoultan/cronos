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
  | 'bar' | 'line' | 'area' | 'pie' | 'donut'
  | 'scatter' | 'bubble' | 'map'
  | 'combo' | 'funnel' | 'waterfall' | 'heatmap' | 'gauge' | 'treemap'

/** The tiles that bucket a dimension and fold a measure. */
export const CATEGORICAL: TileKind[] = [
  'bar', 'line', 'area', 'pie', 'donut', 'waterfall', 'heatmap', 'treemap',
]

/** The tiles that read a list of measures rather than one. */
export const METERED: TileKind[] = ['combo', 'funnel']

/** The tiles that need a second dimension to place a value. */
export const GRIDDED: TileKind[] = ['heatmap']

/** The tiles that fold the whole set to one number. */
export const FOLDED: TileKind[] = ['gauge']

/** The tiles that read two measures against each other. */
export const PLOTS: TileKind[] = ['scatter', 'bubble']

/** The tiles that can draw more than one series at once. */
export const MULTI_SERIES: TileKind[] = [
  'bar', 'line', 'area', 'scatter', 'bubble', 'heatmap', 'treemap',
]

/** The tiles a series dimension stacks on rather than drawing beside. */
export const STACKABLE: TileKind[] = ['bar', 'area']

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
  series?: string
  aggregate?: 'sum' | 'count' | 'avg' | 'min' | 'max'
  /** Draws a multi-series bar or area as one stack per bucket. */
  stacked?: boolean
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
  /** What a gauge reads its value against. */
  target?: TileTarget
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
  /** string, number, bool, date or enum. */
  type: string
  /** The permitted values. Enum only. */
  values?: string[]
  bind: Record<string, string>
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

/** What a gauge measures against: a column, or a number. */
export interface TileTarget {
  field?: string
  aggregate?: 'sum' | 'count' | 'avg' | 'min' | 'max'
  value?: number
  label?: string
}

/** The geography a map tile reads. */
export interface TileMap {
  /** Drawn bottom to top. Empty lets the server infer one from the fields. */
  layers?: string[]
  /** The field carrying GeoJSON, for a polygon layer. */
  geometry?: string
  lat?: string
  lon?: string
  toLat?: string
  toLon?: string
  /** An XYZ tile template. Empty draws no basemap, which is the default: a
   *  basemap is a request from the reader's browser to a third party. */
  basemap?: string
  attribution?: string
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
