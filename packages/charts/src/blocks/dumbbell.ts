import type { Axis, ChartBlock, Dumbbell } from '../types'
import { svg, n } from '../svg'
import { canvas, fit, label, measure, sized, type Measure } from '../frame'
import { thin, TICK_PX } from '../axes'
import { chartPanel, host, hue, nothing } from '../chart'
import { el } from '../dom'
import { withTips, type Tips } from '../tip'

const LABEL_PX = 12
const ROW = 30
const DOT = 6
/** Between the end of the scale and the words after it. */
const GAP = 10

/**
 * A dumbbell: each category as two dots on one scale joined by a line — an
 * open one where it ran from, a filled one where it ran to. Last year and
 * this, the target and the outcome, before and after: the line's length is
 * the change, which two bars side by side leave the reader to work out.
 *
 * Rows come largest end first unless the report sorts them; the scale is
 * shared and drawn along the bottom, and beside each row is where it ended
 * and by how much it moved.
 */
export function dumbbellBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const rows = b.dumbbells ?? []
  const x = b.xAxis
  if (!x || rows.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  let entered = false
  sized(at, (width) => {
    const root = draw(width, rows, x, b.ends ?? [], measure(at), tips)
    if (!entered) root.classList.add('enter')
    entered = true
    at.replaceChildren(root)
  })
  const [from, to] = b.ends ?? []
  if (from && to) {
    panel.append(el('div', { class: 'legend', part: 'legend' },
      el('span', { class: 'key' }, el('i', { class: 'swatch from-swatch' }), from),
      el('span', { class: 'key' }, el('i', { class: 'swatch', style: `background: ${hue(0)}` }), to)))
  }
  return panel
}

function draw(width: number, rows: Dumbbell[], axis: Axis, ends: string[], m: Measure, tips: Tips): SVGSVGElement {
  const words = rows.map((r) => `${r.toSaid} (${r.change})`)
  const wordW = Math.ceil(Math.max(...words.map((w) => m(w, LABEL_PX)))) + GAP + 4
  const nameW = Math.min(Math.ceil(Math.max(...rows.map((r) => m(r.label, LABEL_PX)))), width * 0.3)
  const x0 = nameW + 12 + DOT
  const span = Math.max(1, width - x0 - DOT - Math.min(wordW, width * 0.34))
  const x = (v: number) => x0 + span * (axis.max === axis.min ? 0 : (v - axis.min) / (axis.max - axis.min))
  const floor = rows.length * ROW
  const height = floor + 22

  const root = canvas(width, height)
  const grid = svg('g', { class: 'gridlines' })
  const marks = svg('g', { class: 'marks' })
  const text = svg('g', { class: 'labels' })
  root.append(grid, marks, text)

  const room = span / Math.max(axis.ticks.length - 1, 1)
  const every = thin(Math.max(0, ...axis.ticks.map((t) => m(t.label, TICK_PX))), room)
  axis.ticks.forEach((t, i) => {
    const at = x0 + span * t.at
    grid.append(svg('line', { class: 'gridline', x1: n(at), x2: n(at), y1: 0, y2: n(floor) }))
    if (i % every === 0) text.append(label(at, floor + 16, t.label, 'tick'))
  })

  const [fromName, toName] = ends
  rows.forEach((r, i) => {
    const mid = i * ROW + ROW / 2
    text.append(label(0, mid + 4, fit(r.label, nameW, LABEL_PX, m), 'name', 'start'))
    const a = x(r.from)
    const b = x(r.to)
    const said = `${fromName ?? 'From'} ${r.fromSaid} → ${toName ?? 'to'} ${r.toSaid} (${r.change})`
    const link = svg('line', { class: 'dumbbell-link', x1: n(a), x2: n(b), y1: n(mid), y2: n(mid) })
    const from = svg('circle', { class: 'dumbbell-from', part: 'dot', cx: n(a), cy: n(mid), r: DOT - 1 })
    const to = svg('circle', { class: 'dumbbell-to', part: 'dot', cx: n(b), cy: n(mid), r: DOT })
    to.style.fill = hue(0)
    for (const mark of [link, from, to]) tips.bind(mark, r.label, said)
    marks.append(link, from, to)
    const at = x0 + span + DOT + GAP
    text.append(label(at, mid + 4, fit(words[i] ?? '', width - at - 2, LABEL_PX, m), 'name', 'start'))
  })
  return root
}
