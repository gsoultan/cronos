import type { ChartBlock, Point } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'
import { plot, on } from '../plot'
import { withTips } from '../tip'
import { slotOf, PLOT_PALETTE_SIZE } from '../palette'

const HEIGHT = 62

/**
 * A scatter, or a bubble when the dots carry a size.
 *
 * Unlike the line chart this does not stretch to its panel: a circle in a
 * coordinate space scaled unevenly is an ellipse, and an ellipse encodes a
 * direction the data does not have. It scales uniformly and letterboxes in a
 * very wide panel, which costs some width and keeps every dot round.
 *
 * Area, not radius, carries the size. Radius squares the difference — a value
 * twice another drawn at twice the radius covers four times the ink and reads
 * as four times as much.
 */
export function scatterBlock(b: ChartBlock): HTMLElement {
  const panel = el('section', { class: 'panel wide', part: 'panel' }, el('h3', {}, b.title))
  const tips = withTips(panel)

  const points = b.points ?? []
  const x = b.xAxis
  const y = b.yAxis
  if (!x || !y || points.length === 0) {
    panel.append(el('p', { class: 'unaffected' }, 'No data in this period.'))
    return panel
  }

  const p = plot(x, y, HEIGHT, false)
  for (const dot of points) {
    const [cx, cy] = p.at(on(x, dot.x), on(y, dot.y))
    const mark = svg('circle', {
      cx: n(cx), cy: n(cy), r: n(radius(dot)),
      class: 'dot', part: 'dot',
      fill: `var(--cr-series-${slotOf(dot.slot ?? 0, PLOT_PALETTE_SIZE)})`,
    })
    tips.bind(mark, dot.label, detail(dot))
    p.canvas.append(mark)
  }

  panel.append(p.frame)
  return panel
}

/**
 * The radius for one dot.
 *
 * A floor of 2.2 user units, because a bubble scaled to a weight near zero
 * disappears — and a row that rendered as nothing is indistinguishable from a
 * row that was filtered out. A scatter has no weight at all and every dot
 * takes the same size, which is what makes it a scatter.
 */
function radius(dot: Point): number {
  if (dot.weight === undefined) return 1.6
  return 2.2 + Math.sqrt(dot.weight) * 4.4
}

function detail(dot: Point): string {
  return dot.size ? `${dot.fx} · ${dot.fy} · ${dot.size}` : `${dot.fx} · ${dot.fy}`
}
