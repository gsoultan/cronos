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
  format?: 'currency' | 'percent' | 'number'
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

export type TileKind = 'stat' | 'bar' | 'line' | 'table'

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
  columns?: string[]
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
