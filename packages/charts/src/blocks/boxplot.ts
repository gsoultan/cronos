import type { Box, ChartBlock } from '../types'
import { svg, n } from '../svg'
import { cartesian, type Cartesian } from '../axes'
import { measure, sized } from '../frame'
import { chartPanel, host, hue, nothing, PLOT_HEIGHT } from '../chart'
import { withTips, type Tips } from '../tip'

/**
 * A box plot: for each category, the middle half of its rows as a box, the
 * median a firm line across it, and whiskers to the furthest rows inside
 * Tukey's fences — how a number spreads, which a sum of it cannot say.
 *
 * Rows beyond the whiskers are counted in the tooltip rather than drawn one
 * by one: a dot per row would be a payload per row. The scale is over where
 * the boxes are rather than from nothing, because a box is read by where it
 * sits, not by how tall it stands.
 */
export function boxplotBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const boxes = b.boxes ?? []
  const y = b.yAxis
  if (!y || boxes.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  let entered = false
  sized(at, (width) => {
    const c = cartesian({ width, height: PLOT_HEIGHT, m: measure(at), y,
      categories: boxes.map((box) => box.label || b.title) })
    boxes.forEach((box, i) => drawBox(c, box, i, tips))
    if (!entered) c.svg.classList.add('enter')
    entered = true
    at.replaceChildren(c.svg)
  })
  return panel
}

function drawBox(c: Cartesian, box: Box, i: number, tips: Tips) {
  const band = c.band(i)
  const w = Math.min(band.w * 0.5, 56)
  const mid = band.x + band.w / 2
  const low = c.y(box.low)
  const q1 = c.y(box.q1)
  const median = c.y(box.median)
  const q3 = c.y(box.q3)
  const high = c.y(box.high)
  const line = (x1: number, y1: number, x2: number, y2: number, cls: string) =>
    svg('line', { class: cls, x1: n(x1), y1: n(y1), x2: n(x2), y2: n(y2) })
  const body = svg('rect', { class: 'box-body', x: n(mid - w / 2), y: n(q3), width: n(w),
    height: n(Math.max(q1 - q3, 1)), rx: 3 })
  body.style.fill = hue(0)
  body.style.stroke = hue(0)
  const across = line(mid - w / 2, median, mid + w / 2, median, 'median')
  across.style.stroke = hue(0)
  const g = svg('g', { class: 'box', part: 'box' })
  g.append(
    line(mid, high, mid, q3, 'whisker'), line(mid, q1, mid, low, 'whisker'),
    line(mid - w / 4, high, mid + w / 4, high, 'whisker'), line(mid - w / 4, low, mid + w / 4, low, 'whisker'),
    body, across)
  tips.bind(g, box.label || 'Every row', said(box))
  c.marks.append(g)
}

/** A box's five numbers as a reader asks for them: the middle first. */
function said(box: Box): string {
  const [low, q1, median, q3, high] = box.said
  const beyond = box.outliers ? ` · ${box.outliers} beyond them` : ''
  return `median ${median} · middle half ${q1} to ${q3} · whiskers ${low} to ${high}${beyond} · ${box.n} rows`
}
