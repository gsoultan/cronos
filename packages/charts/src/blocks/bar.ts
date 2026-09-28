import type { Bar, ChartBlock, Group } from '../types'
import { svg, n } from '../svg'
import { bar, canvas, fit, label, measure, sized, type Measure } from '../frame'
import { chartPanel, host, hue, nothing } from '../chart'
import { legend } from '../legend'
import { withTips, type Tips } from '../tip'

const LABEL_PX = 12

/**
 * A horizontal bar chart: a category down the left, its bar, and its value
 * at the bar's end.
 *
 * No value axis. The number beside each bar is the one the engine formatted,
 * so a scale under the bars would only restate it less exactly; the bars rank
 * and the labels state. Where the data goes below nothing, a line marks zero
 * and those bars grow left from it — a bar drawn against the largest value
 * only, as these were, drew a loss as a two-pixel sliver.
 *
 * Stacked, a bucket's parts sit end to end with the surface showing between
 * them, positive parts to the right of zero and negative ones to the left;
 * grouped, each series gets a thinner bar of its own under the bucket.
 */
export function barBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const groups: Group[] = b.groups ?? [{ label: b.title, slot: 0, bars: b.series ?? [] }]
  const buckets = groups[0]?.bars ?? []
  if (buckets.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  const shown = groups.map(() => true)
  let entered = false
  const redraw = sized(at, (width) => {
    const svgEl = draw(width, b, groups, shown, measure(at), tips)
    if (!entered) svgEl.classList.add('enter')
    entered = true
    at.replaceChildren(svgEl)
  })
  const key = legend(groups, (i, on) => { shown[i] = on; redraw() })
  if (key) panel.append(key)
  return panel
}

function draw(width: number, b: ChartBlock, groups: Group[], shown: boolean[], m: Measure,
  tips: Tips): SVGSVGElement {
  const live = groups.filter((_, i) => shown[i])
  const buckets = groups[0]?.bars ?? []
  const stacked = !!b.stacked && groups.length > 1
  const lanes = stacked ? 1 : Math.max(live.length, 1)
  const thick = lanes > 1 ? 12 : 20
  const row = lanes * thick + (lanes - 1) * 3 + 12
  const height = buckets.length * row + 4

  // Where nothing is, and how far a unit reaches, across the room left once
  // the names and the numbers have theirs.
  const ends = buckets.flatMap((_, i) => stacked ? stackEnds(live, i) : live.map((g) => g.bars[i]?.value ?? 0))
  const lo = Math.min(0, ...ends)
  const hi = Math.max(0, ...ends, lo === 0 ? 1e-9 : 0)
  const values = stacked ? (b.totals ?? []).map((t) => t.formatted) : live.flatMap((g) => g.bars.map((x) => x.formatted))
  const valueW = Math.ceil(Math.max(0, ...values.map((v) => m(v, LABEL_PX)))) + 8
  const nameW = Math.min(Math.ceil(Math.max(0, ...buckets.map((x) => m(x.label, LABEL_PX)))), width * 0.36)
  const x0 = nameW + 12 + (lo < 0 ? valueW : 0)
  const span = Math.max(1, width - x0 - valueW)
  const x = (v: number) => x0 + ((v - lo) / (hi - lo)) * span

  const root = canvas(width, height)
  const marks = svg('g', { class: 'marks' })
  const text = svg('g', { class: 'labels' })
  root.append(marks, text)
  if (lo < 0) root.prepend(svg('line', { class: 'gridline zero', x1: n(x(0)), x2: n(x(0)), y1: 0, y2: height }))

  buckets.forEach((bucket, i) => {
    const top = i * row + 6
    text.append(label(0, top + (row - 12) / 2 + 4, fit(bucket.label, nameW, LABEL_PX, m), 'name', 'start'))
    if (stacked) {
      stack(marks, live, i, top, thick, x, tips, bucket.label)
      // Past whichever end the stack reaches on the total's side: parts
      // either side of nothing can net to a value inside the stack.
      const total = b.totals?.[i]
      const [up, down] = stackEnds(live, i)
      if (total) value(text, x, total.value < 0 ? (down ?? 0) : (up ?? 0), total.formatted, top + thick / 2)
      return
    }
    live.forEach((g, k) => {
      const v = g.bars[i]
      if (!v) return
      const y = top + k * (thick + 3)
      mark(marks, x(Math.min(0, v.value)), y, Math.abs(x(v.value) - x(0)), thick, v.value, g.slot, tips,
        live.length > 1 ? `${g.label} · ${bucket.label}` : bucket.label, v)
      value(text, x, v.value, v.formatted, y + thick / 2)
    })
  })
  return root
}

/** The ends a bucket's stack reaches, either side of nothing. */
function stackEnds(groups: Group[], i: number): number[] {
  let up = 0
  let down = 0
  for (const g of groups) {
    const v = g.bars[i]?.value ?? 0
    if (v >= 0) up += v
    else down += v
  }
  return [up, down]
}

/** One bucket's parts, end to end outward from nothing. */
function stack(into: SVGGElement, groups: Group[], i: number, top: number, thick: number,
  x: (v: number) => number, tips: Tips, bucket: string) {
  let up = 0
  let down = 0
  for (const g of groups) {
    const v = g.bars[i]
    if (!v || v.value === 0) continue
    const from = v.value >= 0 ? up : down + v.value
    if (v.value >= 0) up += v.value
    else down += v.value
    // Square between parts, with a pixel of surface either side: two fills
    // that touch read as one fill.
    const left = x(from) + 1
    const w = Math.abs(x(from + Math.abs(v.value)) - x(from)) - 2
    mark(into, left, top, w, thick, v.value, g.slot, tips, `${g.label} · ${bucket}`, v, 1)
  }
}

function mark(into: SVGGElement, x: number, y: number, w: number, h: number, v: number, slot: number,
  tips: Tips, name: string, datum: Bar, radius = 4) {
  // A value of nothing still shows, as a sliver: a bar that is not there
  // reads as a row that was filtered out.
  const path = svg('path', { class: 'fill', part: 'bar', d: bar(x, y, Math.max(w, 2), h, v < 0 ? 'left' : 'right', radius) })
  path.style.fill = hue(slot)
  tips.bind(path, name, datum.formatted)
  into.append(path)
}

/** A bar's number, just past its end: right of a gain, left of a loss. */
function value(into: SVGGElement, x: (v: number) => number, v: number, text: string, mid: number) {
  into.append(v < 0
    ? label(x(v) - 6, mid + 4, text, 'value', 'end')
    : label(x(v) + 6, mid + 4, text, 'value', 'start'))
}
