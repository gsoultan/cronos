import type { ChartBlock, Step } from '../types'
import { svg, n } from '../svg'
import { cartesian } from '../axes'
import { bar, label, measure, sized } from '../frame'
import { chartPanel, host, nothing, PLOT_HEIGHT } from '../chart'
import { withTips } from '../tip'

/**
 * A waterfall: floating columns that each start where the last one ended,
 * joined by a thread at the running total, each with its own change written
 * over it — the number a waterfall is read for, which used to be a tooltip.
 *
 * The running total is the server's — see run.Step. A viewer accumulating it
 * itself would drift from the PDF of the same report by whatever the two
 * languages' float addition disagreed about, which is exactly the kind of
 * difference nobody can explain to a finance team.
 *
 * Coloured by sign from the diverging pair, and deliberately not green and
 * red: that is the one pair a colourblind reader cannot separate, on the one
 * chart whose entire point is which side of nothing a column falls. A total
 * stands on the axis in the neutral, because it is neither.
 */
export function waterfallBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const steps = b.steps ?? []
  const y = b.yAxis
  if (!y || steps.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  let entered = false
  sized(at, (width) => {
    const c = cartesian({
      width, height: PLOT_HEIGHT, m: measure(at), y, categories: steps.map((s) => s.label), headroom: 20,
    })
    steps.forEach((s, i) => {
      const band = c.band(i)
      const w = Math.min(band.w * 0.64, 72)
      const left = band.x + (band.w - w) / 2
      const top = c.y(Math.max(s.start, s.end))
      const bottom = c.y(Math.min(s.start, s.end))
      const col = svg('path', {
        class: 'col', part: 'bar',
        d: bar(left, top, w, Math.max(bottom - top, 1.5), s.sign < 0 && !s.total ? 'down' : 'up', 3),
      })
      col.style.fill = fill(s)
      tips.bind(col, s.label, s.formatted)
      c.marks.append(col)
      // Over a rise and a total, under a fall: where the column's value is.
      const down = s.sign < 0 && !s.total
      c.marks.append(label(left + w / 2, down ? bottom + 14 : top - 6, s.formatted, 'value'))

      const next = steps[i + 1]
      if (!next) return
      const nb = c.band(i + 1)
      const nw = Math.min(nb.w * 0.64, 72)
      c.marks.append(svg('line', {
        class: 'thread', x1: n(left + w), x2: n(nb.x + (nb.w - nw) / 2), y1: n(c.y(s.end)), y2: n(c.y(s.end)),
      }))
    })
    if (!entered) c.svg.classList.add('enter')
    entered = true
    at.replaceChildren(c.svg)
  })
  return panel
}

/** A rise, a fall, or a total — which is neither. */
function fill(s: Step): string {
  if (s.total) return 'var(--cr-neutral)'
  return s.sign < 0 ? 'var(--cr-down)' : 'var(--cr-up)'
}
