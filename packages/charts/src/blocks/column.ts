import type { Axis, Bar, ChartBlock, Group } from '../types'
import { svg } from '../svg'
import { cartesian, type Cartesian } from '../axes'
import { bar, label, measure, sized, type Measure } from '../frame'
import { chartPanel, host, hue, nothing, PLOT_HEIGHT } from '../chart'
import { legend } from '../legend'
import { withTips, type Tips } from '../tip'

const VALUE_PX = 11

/**
 * A column chart: the bar chart stood up, for categories that read left to
 * right — months, weeks, the steps of something.
 *
 * Its scale is down the left, and each column carries its figure over its top
 * while the band has room for it; where the figures would run into each other
 * they are left to the scale and the tooltip. Grouped, each series takes a
 * lane of the band. Stacked, a bucket's parts stand end to end either side of
 * nothing with the total over the top — while every series is showing, since
 * a total of parts somebody has hidden is not the height drawn. Stacked to a
 * whole, each part is its share of its bucket, worked out here from the parts
 * on show, so hiding one gives the others its room rather than leaving a gap.
 */
export function columnBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const groups: Group[] = b.groups ?? [{ label: b.title, slot: 0, bars: b.series ?? [] }]
  const buckets = groups[0]?.bars ?? []
  const y = b.yAxis
  if (!y || buckets.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  const shown = groups.map(() => true)
  let entered = false
  const redraw = sized(at, (width) => {
    const m = measure(at)
    const c = cartesian({ width, height: PLOT_HEIGHT, m, y, categories: buckets.map((x) => x.label), headroom: 18 })
    const words = svg('g', { class: 'labels' })
    const live = groups.filter((_, i) => shown[i])
    const all = live.length === groups.length
    if (b.stacked && groups.length > 1) stacks(c, words, b, live, all, tips, m)
    else lanes(c, words, y, live, groups.length > 1, tips, m)
    c.svg.append(words)
    if (!entered) c.svg.classList.add('enter')
    entered = true
    at.replaceChildren(c.svg)
  })
  const key = legend(groups, (i, on) => { shown[i] = on; redraw() })
  if (key) panel.append(key)
  return panel
}

/** Each series in a lane of its own inside every band. */
function lanes(c: Cartesian, words: SVGGElement, y: Axis, live: Group[], named: boolean, tips: Tips, m: Measure) {
  const count = Math.max(live.length, 1)
  // Where nothing is on the scale, or the end of it nearest nothing.
  const zero = c.y(Math.min(Math.max(0, y.min), y.max))
  live.forEach((g, k) => {
    g.bars.forEach((v, i) => {
      // A series with nothing in this bucket contributes no mark: the server
      // pads a missing pair with zero, and a sliver reads as a small amount.
      if (named && v.value === 0) return
      const band = c.band(i)
      // Capped: three months across a dashboard made columns a third of it
      // wide, which reads as blocks of colour rather than heights.
      const lane = Math.min((band.w * 0.72) / count, 56)
      const left = band.x + (band.w - lane * count) / 2 + k * lane
      const top = c.y(v.value)
      column(c, left + 1, Math.min(top, zero), Math.max(lane - 2, 1), Math.abs(zero - top), v.value, g.slot,
        tips, named ? `${g.label} · ${v.label}` : v.label, v.formatted)
      if (m(v.formatted, VALUE_PX) <= lane + 6) figure(words, left + lane / 2, v.value < 0 ? Math.max(top, zero) + 14 : Math.min(top, zero) - 5, v.formatted)
    })
  })
}

/** A bucket's parts end to end either side of nothing, or as shares of it. */
function stacks(c: Cartesian, words: SVGGElement, b: ChartBlock, live: Group[], all: boolean, tips: Tips, m: Measure) {
  const count = live[0]?.bars.length ?? 0
  for (let i = 0; i < count; i++) {
    const band = c.band(i)
    const w = Math.min(band.w * 0.64, 56)
    const left = band.x + (band.w - w) / 2
    const whole = live.reduce((sum, g) => sum + Math.abs(g.bars[i]?.value ?? 0), 0)
    const size = (v: Bar) => (b.percent ? (whole > 0 ? v.value / whole : 0) : v.value)
    let up = 0
    let down = 0
    for (const g of live) {
      const v = g.bars[i]
      if (!v || v.value === 0) continue
      const s = size(v)
      const from = s >= 0 ? up : down + s
      if (s >= 0) up += s
      else down += s
      const top = c.y(from + Math.abs(s))
      // Square between parts, with a pixel of surface either side: two fills
      // that touch read as one fill.
      column(c, left, top + 1, w, c.y(from) - top - 2, 1, g.slot, tips, `${g.label} · ${v.label}`,
        b.percent ? `${v.formatted} · ${percent(s)}` : v.formatted, 1)
    }
    const total = b.totals?.[i]
    if (all && total && m(total.formatted, VALUE_PX) <= band.w + 6) {
      figure(words, left + w / 2, total.value < 0 && !b.percent ? c.y(down) + 14 : c.y(up) - 5, total.formatted)
    }
  }
}

function column(c: Cartesian, x: number, y: number, w: number, h: number, v: number, slot: number,
  tips: Tips, name: string, text: string, radius = 4) {
  // A value of nothing still shows, as a sliver: a column that is not there
  // reads as a month that was filtered out.
  const path = svg('path', { class: 'col', part: 'bar', d: bar(x, y, w, Math.max(h, 1), v < 0 ? 'down' : 'up', radius) })
  path.style.fill = hue(slot)
  tips.bind(path, name, text)
  c.marks.append(path)
}

function figure(into: SVGGElement, x: number, y: number, text: string) {
  into.append(label(x, y, text, 'value'))
}

function percent(share: number): string {
  return `${Math.round(share * 1000) / 10}%`
}
