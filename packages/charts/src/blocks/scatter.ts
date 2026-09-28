import type { ChartBlock, Point } from '../types'
import { svg, n } from '../svg'
import { cartesian } from '../axes'
import { measure, sized } from '../frame'
import { chartPanel, host, hue, nothing, PLOT_HEIGHT } from '../chart'
import { withTips } from '../tip'
import { PLOT_PALETTE_SIZE } from '../palette'

/**
 * A scatter, or a bubble when the dots carry a size.
 *
 * Drawn at the panel's own size, both axes measured, so a circle is a circle
 * at every width. It used to be drawn in a square and letterboxed into a wide
 * panel, which put the dots in a strip down the middle while the axis under
 * them ran the whole width.
 *
 * Area, not radius, carries a bubble's size — radius squares the difference,
 * and a value twice another drawn at twice the radius reads as four times as
 * much. The largest is sized to the plot rather than to a constant, and the
 * bubbles are drawn largest first so a small one is never hidden under a big
 * neighbour.
 */
export function scatterBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const points = b.points ?? []
  const x = b.xAxis
  const y = b.yAxis
  if (!x || !y || points.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  const sizedPoints = points.some((p) => p.weight !== undefined)
  const order = sizedPoints ? [...points].sort((p, q) => (q.weight ?? 0) - (p.weight ?? 0)) : points
  let entered = false
  sized(at, (width) => {
    const c = cartesian({ width, height: PLOT_HEIGHT, m: measure(at), y, x })
    const most = Math.max(10, Math.min(28, c.plot.h / 7))
    for (const p of order) {
      const dot = svg('circle', {
        class: 'dot', part: 'dot', cx: n(c.x(p.x)), cy: n(c.y(p.y)), r: n(radius(p, most)),
      })
      dot.style.fill = hue(p.slot ?? 0, PLOT_PALETTE_SIZE)
      tips.bind(dot, p.label, detail(p))
      c.marks.append(dot)
    }
    if (!entered) c.svg.classList.add('enter')
    entered = true
    at.replaceChildren(c.svg)
  })
  return panel
}

/**
 * A dot's radius. A scatter's dots are one size, which is what makes it a
 * scatter; a bubble's floor keeps a near-zero value visible, because a row
 * drawn as nothing reads as a row filtered out.
 */
function radius(p: Point, most: number): number {
  if (p.weight === undefined) return 4.5
  return 3.5 + Math.sqrt(Math.max(p.weight, 0)) * (most - 3.5)
}

function detail(p: Point): string {
  return p.size ? `${p.fx} · ${p.fy} · ${p.size}` : `${p.fx} · ${p.fy}`
}
