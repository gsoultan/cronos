/**
 * The wire contract.
 *
 * Every displayable value arrives as a string, already formatted. That is not
 * laziness about types — it is the budget. Formatting money and dates in the
 * browser means shipping locale rules and currency data to every one of an
 * ISV's end users, and it means the number in an embedded tile can disagree
 * with the number in the PDF of the same report. The engine that knew the
 * currency formats it once, for both.
 */

export interface Delta {
  /** Already formatted, e.g. "+6.4%". */
  value: string
  dir: 'up' | 'down'
  /** Whether this direction is good news. Outstanding rising is not. */
  good: boolean
  label?: string
}

/**
 * Which shared filters reached this block's dataset.
 *
 * The report format promises a block says when a filter does not apply to it.
 * The server computes this (it is the only thing that can), and the component
 * renders it — see coverage.ts.
 */
export interface Coverage {
  applied?: string[]
  ignored?: string[]
}

export interface StatBlock {
  kind: 'stat'
  title: string
  value: string
  delta?: Delta
  coverage?: Coverage
}

export interface BarBlock {
  kind: 'bar'
  title: string
  /** Bars in the order they should be drawn. Values are numbers because the
   *  chart has to compare them; labels carry the formatted text. */
  series: { label: string; value: number; formatted: string }[]
  coverage?: Coverage
}

export interface TableBlock {
  kind: 'table'
  title: string
  columns: { label: string; align?: 'left' | 'right' }[]
  rows: string[][]
  /** Total matching rows, which may exceed the page returned. */
  total?: number
  coverage?: Coverage
}

export type Block = StatBlock | BarBlock | TableBlock

export interface FilterDef {
  name: string
  label: string
  type: string
}

export interface ReportPayload {
  title: string
  description?: string
  filters?: FilterDef[]
  blocks: Block[]
}

/** What a host page sets to narrow the report. */
export type FilterValues = Record<string, { op: string; values: unknown[] }>
