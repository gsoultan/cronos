import type { Axis, Bullet, ChartBlock } from '../types'
import { svg, n } from '../svg'
import { bar, canvas, fit, label, measure, sized, type Measure } from '../frame'
import { thin, TICK_PX } from '../axes'
import { chartPanel, host, hue, nothing } from '../chart'
import { withTips, type Tips } from '../tip'

const LABEL_PX = 12
const ROW = 34
/** Between the end of the scale and the words after it. */
const GAP = 10

/**
 * Bullets: each category's value as a bar across a track shaded by how far
 * it is towards its target, and the target a mark across the bar.
 *
 * A gauge's reading in a row's height, so twenty of them fit where one dial
 * would — which is the reason the form exists. The shades are the target's
 * fractions the report names, darkest furthest from it; the scale is shared
 * by every row, so a longer bar is a larger value, and is drawn along the
 * bottom. Beside each, the value, the target and how far along it is.
 */
export function bulletBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const rows = b.bullets ?? []
  const x = b.xAxis
  if (!x || rows.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  let entered = false
  sized(at, (width) => {
    const root = draw(width, rows, x, b.bands ?? [0.6, 0.9], measure(at), tips)
    if (!entered) root.classList.add('enter')
    entered = true
    at.replaceChildren(root)
  })
  return panel
}

function draw(width: number, rows: Bullet[], axis: Axis, bands: number[], m: Measure, tips: Tips): SVGSVGElement {
  // Measured at the size they are drawn in, with the gap before them counted
  // once: measured a size small and a gap short, "394.8%" ran off the panel.
  const words = rows.map((r) => `${r.formatted} of ${r.targetFormatted} · ${share(r)}`)
  const wordW = Math.ceil(Math.max(...words.map((w) => m(w, LABEL_PX)))) + GAP + 4
  const nameW = Math.min(Math.ceil(Math.max(...rows.map((r) => m(r.label, LABEL_PX)))), width * 0.3)
  const x0 = nameW + 12
  const span = Math.max(1, width - x0 - Math.min(wordW, width * 0.34))
  const x = (v: number) => x0 + span * (axis.max === axis.min ? 0 : (v - axis.min) / (axis.max - axis.min))
  const height = rows.length * ROW + 22

  const root = canvas(width, height)
  const marks = svg('g', { class: 'marks' })
  const text = svg('g', { class: 'labels' })
  root.append(marks, text)
  rows.forEach((r, i) => {
    const top = i * ROW + 6
    const mid = top + (ROW - 12) / 2
    text.append(label(0, mid + 4, fit(r.label, nameW, LABEL_PX, m), 'name', 'start'))
    track(marks, r, bands, axis, x, top, ROW - 12)
    const path = svg('path', { class: 'fill', part: 'bar',
      d: bar(Math.min(x(0), x(r.value)), mid - 4, Math.max(Math.abs(x(r.value) - x(0)), 2), 8, r.value < 0 ? 'left' : 'right', 2) })
    path.style.fill = hue(0)
    tips.bind(path, r.label, `${r.formatted} of ${r.targetFormatted} · ${share(r)}`)
    marks.append(path, svg('line', { class: 'target', part: 'target',
      x1: n(x(r.target)), x2: n(x(r.target)), y1: n(top + 2), y2: n(top + ROW - 14) }))
    const at = x0 + span + GAP
    text.append(label(at, mid + 4, fit(words[i] ?? '', width - at - 2, LABEL_PX, m), 'name', 'start'))
  })
  scale(text, axis, x, rows.length * ROW + 16, m)
  return root
}

/** The shades behind a bar: the target's fractions, darkest furthest from it. */
function track(into: SVGGElement, r: Bullet, bands: number[], axis: Axis, x: (v: number) => number, top: number, h: number) {
  const edges = [axis.min, ...bands.map((f) => f * r.target), axis.max]
  for (let k = 0; k < edges.length - 1; k++) {
    const from = Math.max(axis.min, Math.min(edges[k] ?? 0, axis.max))
    const to = Math.max(from, Math.min(edges[k + 1] ?? 0, axis.max))
    if (to <= from) continue
    into.append(svg('rect', { class: `shade s${Math.min(k, 2)}`, x: n(x(from)), y: n(top), width: n(x(to) - x(from)), height: n(h) }))
  }
}

/** The shared scale, under the last row. */
function scale(into: SVGGElement, axis: Axis, x: (v: number) => number, y: number, m: Measure) {
  // Every tick where they fit side by side, else every second or third: the
  // track is what is left between the names and the figures, and in a narrow
  // panel five ticks along it were one run of digits.
  const room = Math.abs(x(axis.max) - x(axis.min)) / Math.max(axis.ticks.length - 1, 1)
  const every = thin(Math.max(0, ...axis.ticks.map((t) => m(t.label, TICK_PX))), room)
  axis.ticks.forEach((t, i) => {
    if (i % every !== 0) return
    into.append(label(x(axis.min + t.at * (axis.max - axis.min)), y, t.label, 'tick'))
  })
}

function share(r: Bullet): string {
  return r.target ? `${Math.round((r.value / r.target) * 1000) / 10}%` : ''
}
