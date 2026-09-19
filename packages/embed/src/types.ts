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

/** One point of a chart that draws a single series. */
export interface Bar {
  label: string
  value: number
  formatted: string
}

/** One series of a chart that draws several. */
export interface Group {
  label: string
  /** Which categorical colour slot, from 0. A slot and not a colour: the
   *  palette is a set of CSS custom properties on the host, which is the only
   *  theming API this package has. */
  slot: number
  /** Dense — every group covers every bucket, padded with zeroes. The server
   *  pads so a stacked chart can add a column up without each viewer
   *  re-deriving which buckets a series missed. */
  bars: Bar[]
}

/** One dot of a scatter or bubble chart. */
export interface Point {
  label: string
  x: number
  y: number
  /** x and y, formatted, for the tooltip. */
  fx: string
  fy: string
  /** The size measure scaled 0..1, which is what a bubble's radius reads. */
  weight?: number
  size?: string
  slot?: number
}

export interface Tick {
  /** Where the tick sits, 0 at min and 1 at max. */
  at: number
  label: string
}

export interface Axis {
  min: number
  max: number
  ticks: Tick[]
}

/** The Web Mercator world-unit box a map fits, already padded. */
export interface Bounds {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

/** One polygon of a choropleth. */
export interface Shape {
  label: string
  /** An SVG `d` in world units, projected by the server. */
  path: string
  value: number
  formatted: string
  /** Which stop of the sequential ramp shades it, from 0 for the lightest. */
  step: number
}

/** One point on a map — a dot, a bubble, or a contribution to a heat field. */
export interface Marker {
  label: string
  x: number
  y: number
  value: number
  formatted: string
  /** The value scaled 0..1 across the map. */
  weight: number
  size?: string
}

/** One flow, from somewhere to somewhere else. */
export interface Arc {
  label: string
  x1: number
  y1: number
  x2: number
  y2: number
  value: number
  formatted: string
  weight: number
}

/** One band of a map's ramp, with the values it covers already formatted. */
export interface LegendStop {
  step: number
  from: string
  to: string
}

/** The basemap under a map's data. Absent unless the author asked for one. */
export interface Tiles {
  /** An XYZ template the viewer fills in per tile. */
  url: string
  /** The credit line, which every tile source requires be displayed. */
  attribution: string
  maxZoom: number
}

export interface GeoMap {
  bounds: Bounds
  /** What to draw, bottom to top. A viewer draws what it knows and ignores
   *  the rest, which is how a map gains a layer without every pinned copy of
   *  this bundle breaking. */
  layers: string[]
  shapes: Shape[]
  markers: Marker[]
  arcs: Arc[]
  legend: LegendStop[]
  tiles?: Tiles
}

/** One measure of a combo chart, and how it is drawn. */
export interface Track {
  label: string
  slot: number
  draw: 'bar' | 'line'
  /** Reads against its own scale. Per measure, never a property of the chart:
   *  an author opts one measure out of the shared scale, rather than opting
   *  the chart into having two. */
  secondary?: boolean
  /** Dense — every track covers every bucket, in the same order. */
  bars: Bar[]
}

/** One step of a funnel. */
export interface Stage {
  label: string
  value: number
  formatted: string
  /** Share of the first stage, 0..1 — the width of the band. */
  share: number
  /** The fall from the previous stage, formatted. Absent on the first, which
   *  has nothing to have fallen from. */
  drop?: string
}

/** One bar of a waterfall, floating between two running totals. */
export interface Step {
  label: string
  value: number
  formatted: string
  start: number
  end: number
  /** -1, 0 or 1. Sent rather than derived: the closing bar has a sign of zero
   *  while carrying a positive value, and comparing against zero would paint
   *  it as a rise. */
  sign: number
  total?: boolean
}

/** One square of a heatmap. */
export interface Cell {
  row: string
  column: string
  value: number
  formatted: string
  step: number
  /** A pair no row matched. Drawn as absence rather than as the lightest
   *  shade — "none" and "nearly none" are different answers. */
  empty?: boolean
}

/** One number read against a target. */
export interface Gauge {
  value: number
  formatted: string
  target: number
  targetFormatted: string
  targetLabel: string
  /** 0..1, capped: an arc cannot draw 180% without wrapping past its own
   *  start and reading as 80%. */
  share: number
  /** How far past the target, formatted — what capping the arc loses. */
  over?: string
}

/** One rectangle of a treemap, already laid out by the server. */
export interface Rect {
  label: string
  value: number
  formatted: string
  /** Fractions of the whole map, so a viewer scales rather than re-lays out. */
  x: number
  y: number
  w: number
  h: number
  group?: string
  slot: number
  /** 0 for a group's frame, 1 for a leaf inside it. */
  depth: number
}

export interface ChartBlock {
  kind: 'chart'
  /** Which chart. A new type here is not a new block kind. */
  chart: string
  title: string
  /** Points in the order they should be drawn, for the types that draw one
   *  series. Always present — empty, not absent, on the types that carry their
   *  points in one of the fields below instead. */
  series: Bar[]
  /** One entry per series, when the block splits by a dimension. */
  groups?: Group[]
  stacked?: boolean
  /** The height of each stack, formatted by the engine that knew the
   *  currency. Present when stacked. */
  totals?: Bar[]
  points?: Point[]
  xAxis?: Axis
  yAxis?: Axis
  map?: GeoMap
  /** A combo chart's measures, and the second scale when one opted out. */
  tracks?: Track[]
  axis2?: Axis
  stages?: Stage[]
  steps?: Step[]
  cells?: Cell[]
  heatRows?: string[]
  heatColumns?: string[]
  rects?: Rect[]
  gauge?: Gauge
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

export type Block = StatBlock | ChartBlock | TableBlock

export interface FilterDef {
  name: string
  label: string
  /** string, number, bool, date or enum. */
  type: string
  /** The permitted values. Enum only, and what the built-in control lists. */
  values?: string[]
  /** Which interface to operate it through, already resolved by the server to
   *  the type's default. Resolved there so this viewer and the portal cannot
   *  disagree about what an author who chose nothing gets. */
  control?: string
}

export interface ReportPayload {
  title: string
  description?: string
  filters?: FilterDef[]
  blocks: Block[]
}

/** What a host page sets to narrow the report. */
export type FilterValues = Record<string, { op: string; values: unknown[] }>
