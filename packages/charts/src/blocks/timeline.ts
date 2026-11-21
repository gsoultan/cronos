import type { Axis, ChartBlock, RefLine, Span } from '../types'
import { svg, n } from '../svg'
import { canvas, fit, label, measure, sized, type Measure } from '../frame'
import { thin, TICK_PX } from '../axes'
import { chartPanel, host, hue, nothing } from '../chart'
import { legend } from '../legend'
import { withTips, type Tips } from '../tip'

const LABEL_PX = 12
const ROW = 26

/**
 * A timeline: each row a bar from its start to its end along a calendar —
 * a roadmap, a project plan, who is booked when.
 *
 * Rows are in the order the engine sent them, which is the order they start
 * in unless the report sorted them otherwise, so the plan reads top left to
 * bottom right. A faint rule rises from each month on the scale, because a
 * bar's ends are read against the calendar and a scale only along the bottom
 * is a long way from the first rows. Coloured by series where there is one —
 * a status, a team, a phase — with a key.
 */
export function timelineBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const spans = b.spans ?? []
  const x = b.xAxis
  if (!x || spans.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  let entered = false
  sized(at, (width) => {
    const root = draw(width, spans, x, measure(at), tips, b.today)
    if (!entered) root.classList.add('enter')
    entered = true
    at.replaceChildren(root)
  })
  const series = keys(spans)
  const key = legend(series)
  if (key) panel.append(key)
  return panel
}

function draw(width: number, spans: Span[], axis: Axis, m: Measure, tips: Tips,
  today?: RefLine): SVGSVGElement {
  const nameW = Math.min(Math.ceil(Math.max(...spans.map((s) => m(s.label, LABEL_PX)))), width * 0.32)
  const x0 = nameW + 14
  const plotW = Math.max(1, width - x0 - 6)
  const x = (v: number) => x0 + plotW * (axis.max === axis.min ? 0 : (v - axis.min) / (axis.max - axis.min))
  // As tall as its rows: a plan of forty items is scrolled past, where
  // squeezed into a panel's height its names would not fit their rows.
  const floor = spans.length * ROW
  // Today is named in a band above the rows, where it meets no bar and no
  // month along the bottom.
  const band = today ? 18 : 0
  const height = band + floor + 24

  const root = canvas(width, height)
  const grid = svg('g', { class: 'gridlines' })
  const marks = svg('g', { class: 'marks' })
  const text = svg('g', { class: 'labels' })
  const plot = svg('g', band ? { transform: `translate(0 ${band})` } : {})
  plot.append(grid, marks, text)
  root.append(plot)

  const room = plotW / Math.max(axis.ticks.length - 1, 1)
  const every = thin(Math.max(0, ...axis.ticks.map((t) => m(t.label, TICK_PX))), room)
  axis.ticks.forEach((t, i) => {
    const at = x0 + plotW * t.at
    grid.append(svg('line', { class: 'gridline', x1: n(at), x2: n(at), y1: 0, y2: n(floor) }))
    if (i % every !== 0 || t.at >= 1) return
    // Each month named from where it starts, as a calendar does, and held
    // inside the chart at the right.
    const w = m(t.label, TICK_PX)
    text.append(label(Math.min(at + 3, width - w - 2), floor + 16, t.label, 'tick', 'start'))
  })

  spans.forEach((s, i) => {
    const top = i * ROW
    const mid = top + ROW / 2
    text.append(label(0, mid + 4, fit(s.label, nameW, LABEL_PX, m), 'name', 'start'))
    const left = x(s.from)
    if (s.milestone) {
      // A day, not a stretch of days: a diamond on it, which a bar a day
      // wide at a year's scale is not — a sliver nobody sees.
      const cx = x(s.from + 0.5)
      const r = ROW / 2 - 5
      const diamond = svg('path', {
        class: 'span milestone', part: 'bar',
        d: `M${n(cx)} ${n(mid - r)}L${n(cx + r)} ${n(mid)}L${n(cx)} ${n(mid + r)}L${n(cx - r)} ${n(mid)}Z`,
      })
      diamond.style.fill = hue(s.slot)
      tips.bind(diamond, s.series ? `${s.label} · ${s.series}` : s.label, s.said)
      marks.append(diamond)
      return
    }
    const rect = svg('rect', {
      class: 'span', part: 'bar', x: n(left), y: n(top + 6), width: n(Math.max(x(s.to) - left, 3)),
      height: ROW - 12, rx: 3,
    })
    rect.style.fill = hue(s.slot)
    tips.bind(rect, s.series ? `${s.label} · ${s.series}` : s.label, s.said)
    marks.append(rect)
  })
  if (today) {
    // Today, across every row: what is late, under way and still to come,
    // read against one line rather than the reader's own date.
    const at = x(today.value)
    const said = `Today · ${today.formatted}`
    const w = m(said, TICK_PX)
    const left = at + w + 8 > width
    marks.append(svg('line', { class: 'today', x1: n(at), x2: n(at), y1: -4, y2: n(floor) }))
    root.append(label(left ? at - 4 : at + 4, 12, said, 'today-label', left ? 'end' : 'start'))
  }
  return root
}

/** One key per series, in the order they were coloured. */
function keys(spans: Span[]): { label: string; slot: number }[] {
  const seen = new Map<string, number>()
  for (const s of spans) if (s.series && !seen.has(s.series)) seen.set(s.series, s.slot)
  return [...seen].map(([label, slot]) => ({ label, slot }))
}
