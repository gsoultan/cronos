import type { Axis } from './types'
import { svg, n } from './svg'
import { canvas, fit, label, type Measure } from './frame'

/** The size tick labels are drawn at, in pixels. */
export const TICK_PX = 11

/**
 * The frame of a plotted chart: its plot rectangle, a value axis on the left
 * and optionally a second on the right, and along the bottom either one band
 * per category or a measured axis of its own.
 *
 * The margins are measured, not guessed. The left one is as wide as the
 * widest tick label, which was a fixed 3.4rem that clipped "1,250,000" to
 * "250,000"; the category labels are thinned to every second or third where
 * they would run into each other, and each is cut to the room it has, because
 * "May 2026 Jun 2026" squeezed into one band reads as "May 202Bun 202".
 */
export interface Cartesian {
  svg: SVGSVGElement
  /** Where marks go: above the gridlines, below the tooltip's rule. */
  marks: SVGGElement
  /** The plot rectangle, inside the axes. */
  plot: { x: number; y: number; w: number; h: number }
  left: number
  right: number
  top: number
  bottom: number
  /** A value's height in pixels, on the left axis or the one given. */
  y(v: number, axis?: Axis): number
  /** Category i's band: its left edge and width. */
  band(i: number): { x: number; w: number }
  /** A value's position on the bottom axis, where it is measured. */
  x(v: number): number
}

export interface CartesianSpec {
  width: number
  height: number
  m: Measure
  y: Axis
  /** A second scale on the right, its labels in its track's colour. */
  y2?: Axis
  y2Colour?: string
  /** One band per category, labelled with these. */
  categories?: string[]
  /** A measured bottom axis, for a scatter. */
  x?: Axis
  /** Room above the plot for labels drawn over the tallest mark. */
  headroom?: number
}

export function cartesian(s: CartesianSpec): Cartesian {
  const widest = (a?: Axis) => Math.max(0, ...(a?.ticks ?? []).map((t) => s.m(t.label, TICK_PX)))
  const left = Math.ceil(widest(s.y)) + 10
  const lastX = s.x?.ticks[s.x.ticks.length - 1]
  const right = s.y2 ? Math.ceil(widest(s.y2)) + 10
    : lastX ? Math.ceil(s.m(lastX.label, TICK_PX) / 2) + 2 : 6
  const top = s.headroom ?? 10
  const bottom = s.categories || s.x ? 22 : 6
  const plotW = Math.max(1, s.width - left - right)
  const plotH = Math.max(1, s.height - top - bottom)

  const root = canvas(s.width, s.height)
  const grid = svg('g', { class: 'gridlines' })
  const marks = svg('g', { class: 'marks' })
  const labels = svg('g', { class: 'ticks' })
  root.append(grid, marks, labels)

  const on = (a: Axis, v: number) => (a.max === a.min ? 0 : (v - a.min) / (a.max - a.min))
  const y = (v: number, a: Axis = s.y) => top + plotH * (1 - on(a, v))
  const x = (v: number) => left + plotW * (s.x ? on(s.x, v) : 0)
  const count = s.categories?.length ?? 0
  const band = (i: number) => ({ x: left + (plotW / Math.max(count, 1)) * i, w: plotW / Math.max(count, 1) })

  for (const t of s.y.ticks) {
    const at = top + plotH * (1 - t.at)
    // The line at nothing is the one every bar stands on, so it is drawn
    // firmer than the rest, which are there to be measured against.
    const zero = s.y.min + t.at * (s.y.max - s.y.min) === 0 && s.y.min < 0
    grid.append(svg('line', { class: zero ? 'gridline zero' : 'gridline', x1: left, x2: s.width - right, y1: n(at), y2: n(at) }))
    labels.append(label(left - 8, at + 4, t.label, 'tick', 'end'))
  }
  // The floor, whatever the ticks were: a plot with nothing under it floats.
  grid.append(svg('line', { class: 'gridline base', x1: left, x2: s.width - right, y1: n(y(Math.max(s.y.min, 0))), y2: n(y(Math.max(s.y.min, 0))) }))

  for (const t of s.y2?.ticks ?? []) {
    const tick = label(s.width - right + 8, top + plotH * (1 - t.at) + 4, t.label, 'tick', 'start')
    if (s.y2Colour) tick.style.fill = s.y2Colour
    labels.append(tick)
  }

  if (s.categories) categoryLabels(labels, s.categories, band, s.height - 6, s.width, s.m)
  if (s.x) measuredLabels(labels, grid, s.x, left, plotW, top, top + plotH, s.width, s.height - 6, s.m)

  return { svg: root, marks, plot: { x: left, y: top, w: plotW, h: plotH }, left, right, top, bottom, y, band, x }
}

/**
 * One label per band, or per second or third band where they would touch,
 * each cut to the room its share of bands gives it and kept inside the chart
 * at either end. The first always shows: an axis whose labels start at the
 * second category reads as starting there.
 */
function categoryLabels(into: SVGGElement, names: string[], band: Cartesian['band'],
  baseline: number, width: number, m: Measure) {
  const w = band(0).w
  const every = thin(Math.max(0, ...names.map((t) => m(t, TICK_PX))), w)
  names.forEach((name, i) => {
    if (i % every !== 0) return
    const b = band(i)
    const text = fit(name, w * every - 8, TICK_PX, m)
    if (!text) return
    const tw = m(text, TICK_PX)
    const start = Math.max(2, Math.min(b.x + b.w / 2 - tw / 2, width - tw - 2))
    into.append(label(start, baseline, text, 'tick', 'start'))
  })
}

/**
 * How many bands one label spans: every band while the widest label fits in
 * one with room to spare, else every second or third. A label wider than
 * 140px is cut rather than allowed to thin the axis to nothing.
 */
export function thin(widest: number, band: number): number {
  return Math.max(1, Math.ceil((Math.min(widest, 140) + 12) / Math.max(band, 1)))
}

/**
 * A measured bottom axis: a label under each tick and a faint line up from
 * it, the end labels held inside the plot's width rather than centred off it.
 */
function measuredLabels(into: SVGGElement, grid: SVGGElement, a: Axis, left: number, plotW: number,
  top: number, floor: number, width: number, baseline: number, m: Measure) {
  let last = -Infinity
  for (const t of a.ticks) {
    const at = left + plotW * t.at
    const w = m(t.label, TICK_PX)
    const start = Math.max(2, Math.min(at - w / 2, width - w - 2))
    // A label that would run into the one before it is left out; the line
    // above it still marks the tick.
    grid.append(svg('line', { class: 'gridline', x1: n(at), x2: n(at), y1: top, y2: n(floor) }))
    if (start < last + 6) continue
    into.append(label(start, baseline, t.label, 'tick', 'start'))
    last = start + w
  }
}
