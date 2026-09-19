import type { ChartBlock, Step } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'
import { plot, on, PLOT_W } from '../plot'
import { withTips } from '../tip'

const HEIGHT = 58

/**
 * A waterfall: floating bars that each start where the last one ended.
 *
 * The running total is the server's — see run.Step. A viewer accumulating it
 * itself would drift from the PDF of the same report by whatever the two
 * languages' float addition disagreed about, which is exactly the kind of
 * difference nobody can explain to a finance team.
 *
 * Coloured by sign from the diverging pair, and deliberately not green/red:
 * that is the one pair a colourblind reader cannot separate, on the one chart
 * whose entire point is which side of nothing a bar falls.
 */
export function waterfallBlock(b: ChartBlock): HTMLElement {
  const panel = el('section', { class: 'panel wide', part: 'panel' }, el('h3', {}, b.title))
  const steps = b.steps ?? []
  const y = b.yAxis
  if (!y || steps.length === 0) {
    panel.append(el('p', { class: 'unaffected' }, 'No data in this period.'))
    return panel
  }

  const p = plot({ min: 0, max: 1, ticks: ticks(steps) }, y, HEIGHT, true)
  const tips = withTips(panel)
  const slice = PLOT_W / steps.length

  steps.forEach((s, i) => {
    const [, top] = p.at(0, on(y, Math.max(s.start, s.end)))
    const [, base] = p.at(0, on(y, Math.min(s.start, s.end)))
    const rect = svg('rect', {
      x: n(i * slice + slice * 0.15), y: n(top),
      width: n(slice * 0.7), height: n(Math.max(base - top, 0.8)),
      fill: fill(s), class: 'col', part: 'bar',
    })
    tips.bind(rect, s.label, s.formatted)
    p.canvas.append(rect)

    // The thread between one bar and the next, which is what makes the
    // sequence read as one falling total rather than as separate bars.
    if (i < steps.length - 1 && !steps[i + 1]?.total) {
      const [, at] = p.at(0, on(y, s.end))
      p.canvas.append(svg('line', {
        class: 'thread', x1: n(i * slice + slice * 0.85), x2: n((i + 1) * slice + slice * 0.15),
        y1: n(at), y2: n(at),
      }))
    }
  })

  panel.append(p.frame)
  return panel
}

/** A rise, a fall, or the closing total — which is neither. */
function fill(s: Step): string {
  if (s.total) return 'var(--cr-neutral)'
  return s.sign < 0 ? 'var(--cr-down)' : 'var(--cr-up)'
}

function ticks(steps: Step[]) {
  const every = Math.max(1, Math.ceil(steps.length / 6))
  return steps
    .map((s, i) => ({ at: (i + 0.5) / steps.length, label: s.label, i }))
    .filter((t) => t.i % every === 0)
}
