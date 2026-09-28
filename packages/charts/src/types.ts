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

/** One row of a bullet chart: a category's value and the target it is read
 *  against, each formatted by the engine that knew the unit. */
export interface Bullet {
  label: string
  value: number
  formatted: string
  target: number
  targetFormatted: string
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
  /** The region in each period of a map that plays through time — see
   *  GeoMap.frames. The fields above are every period together. */
  frames?: Frames
}

/** One point on a map — a dot, a bubble, a member of a cluster, or a
 *  contribution to a heat field. */
export interface Marker {
  label: string
  x: number
  y: number
  value: number
  formatted: string
  /** The value scaled 0..1 across the map. */
  weight: number
  size?: string
  /** The colour of the point's category, when the map colours by one. */
  slot?: number
  /** How many places this marker stands for: a cell of a large map gathers
   *  several. One when absent. */
  n?: number
  /** The place in each period of a map that plays through time. */
  frames?: Frames
}

/**
 * A mark in each period of a map that plays through time that it had rows in,
 * keyed by the period's place in GeoMap.frames. Keyed rather than listed, so
 * a place seen in one period of sixty is not fifty-nine nulls.
 */
export type Frames = Record<number, FrameMark>

/** A mark's value in one period. One-letter keys: a timed map sends one of
 *  these per mark per period it had rows in. */
export interface FrameMark {
  v: number
  /** The value as a reader reads it. */
  f: string
  /** The ramp step, for a shaded mark; 0 when absent. */
  s?: number
  /** The value scaled 0..1 against every period, for a sized mark; 0 when
   *  absent. */
  w?: number
  /** A place's size measure in the period. */
  z?: string
}

/**
 * A large map's places, gathered into a grid — see run.Cells. Parallel lists
 * rather than a list of objects, because fifty thousand objects is fifty
 * thousand copies of every key name. A cell holding one place is that place.
 */
export interface Cells {
  /** A cell's edge in world units. */
  size: number
  x: number[]
  y: number[]
  /** Places in each cell. */
  n: number[]
  /** The measure folded over them. */
  v: number[]
  /** The bubble's size measure, folded the same way. */
  z?: number[]
  /** Each cell's category slot. */
  s?: number[]
  /** A lone place's label; empty for a cell of several. */
  l?: string[]
  /** v is an average across each cell's places rather than a total. */
  mean?: boolean
}

/** How a viewer asks for more of a large map: see MapViewAsk. */
export interface MapDetail {
  output: string
  block: number
  /** Which dataset drawn over the map, from one; absent is the map itself. */
  overlay?: number
  categories?: string[]
}

/** One view of a large map, asked of the server: where the reader is looking,
 *  in world units as minX, minY, maxX, maxY, and the pixels it is drawn in. */
export interface MapViewAsk {
  output: string
  block: number
  overlay?: number
  view: [number, number, number, number]
  width: number
  height: number
  categories?: string[]
}

/** What answers a view: the host's own route to the server, carrying the
 *  reader's own credentials — the renderer never sees them. */
export type MapViewer = (ask: MapViewAsk, signal: AbortSignal) => Promise<GeoMap>

/** What a host gives the renderers beyond the payload. */
export interface DrawOptions {
  /** Asks for the part of a large map in view. Without it a large map is
   *  drawn from the cells it opened with, at every zoom. */
  mapView?: MapViewer
  /**
   * Sets one of the report's filters — a region clicked, a map's view — or,
   * with null, lets it go. The host owns the filters and re-renders with
   * them; without this a map offers nothing to click.
   */
  filter?: (name: string, value: { op: string; values: unknown[] } | null) => void
}

/** The report filter a click on a map sets — see run.MapPick — and what it
 *  holds now. */
export interface MapPick {
  filter: string
  values?: string[]
}

/** The area filter a map sets from its view — see run.MapArea — and, when
 *  set, the box (south, west, north, east) or the place and distance it holds. */
export interface MapArea {
  filter: string
  op?: 'within' | 'near'
  values?: number[]
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
  slot?: number
  /** The flow in each period of a map that plays through time. */
  frames?: Frames
}

/** One entry of a map's categorical legend. */
export interface MapKey {
  label: string
  slot: number
}

/** One band of a map's ramp, with the values it covers already formatted. */
export interface LegendStop {
  step: number
  from: string
  to: string
}

/** One part of a basemap's credit line, linked where its terms want one. */
export interface Credit {
  text: string
  href?: string
}

/** The basemap under a map's data. Absent unless the author asked for one. */
export interface Tiles {
  /** An XYZ template the viewer fills in per tile. */
  url: string
  /** The same tiles at twice the pixel density, where the source has them. */
  url2x?: string
  /** The credit line, which every tile source requires be displayed. */
  attribution: string
  /** The same line in parts, some of them links. Drawn instead of
   *  `attribution` by a viewer that knows it. */
  credits?: Credit[]
  /** A logo the provider's terms require on the map itself. */
  logo?: string
  logoAlt?: string
  maxZoom: number
  /** A tile's edge in CSS pixels at its own zoom. 256 when absent. */
  tileSize?: number
  /** Where to ask for the credit line of what is in view, with the view
   *  appended — Google's copyright depends on the imagery on screen. */
  viewport?: string
  /** The same provider's tiles for a dark page, when the map follows the
   *  page's theme; drawn under a dark theme in place of these. */
  dark?: Tiles
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
  /** The line layer's routes and the hexbin layer's cells. */
  lines?: Shape[]
  hexes?: Shape[]
  /** What each colour means, when points are coloured by category. */
  keys?: MapKey[]
  /** 'diverging' when the shaded layers take two hues either side of a
   *  midpoint — steps below the middle cool, from it up warm. */
  ramp?: 'diverging'
  /** How far a radius layer's circles reach, in kilometres on the ground. */
  radiusKm?: number
  /** Whether the map names its regions and places on itself. */
  labels?: boolean
  /** Whether its flows move from where each starts to where it lands. */
  animate?: boolean
  /** The periods a map that plays through time plays, earliest first, as a
   *  reader reads them. Its marks as sent are every period together; each
   *  one's `frames` is its value in each. */
  frames?: string[]
  /** What a period's shades mean: one set for every period, so a colour
   *  means one value in all of them. */
  frameLegend?: LegendStop[]
  tiles?: Tiles
  /** Why a basemap the author asked for is not under the data. */
  note?: string
  /** That the map was drawn from the first rows of more than it could hold,
   *  and what that does to its totals. */
  partial?: string
  /** A large map's places, gathered where they crowd together. */
  cells?: Cells
  /** How many places a large map holds. */
  places?: number
  /** How to ask for more of a large map; absent on one that fits. */
  detail?: MapDetail
  /** The report filters this map sets: a click on a region, and its view. */
  pick?: MapPick
  area?: MapArea
  /** Other datasets drawn over this map, bottom to top — each a map of its
   *  own — and, on an overlay, what its legend is headed with. */
  overlays?: GeoMap[]
  title?: string
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
  /** Stacked to a whole: each part drawn as its share of its bucket, against
   *  a scale of percentages. The values are still the values. */
  percent?: boolean
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
  /** A bullet chart's rows, read along `xAxis`, and the fractions of each
   *  row's target its track is shaded at. */
  bullets?: Bullet[]
  bands?: number[]
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

/** Prose on a report: a heading, a note, a caveat beside a number. */
export interface TextBlock {
  kind: 'text'
  title: string
  value: string
  coverage?: Coverage
}

export type Block = StatBlock | ChartBlock | TableBlock | TextBlock

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
