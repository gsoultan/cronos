import type { ChartBlock } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'

/**
 * One number against a target.
 *
 * A stat tile answers "what is it"; this answers "is it enough", and the
 * target is the whole difference. An open arc rather than a full ring, because
 * a ring at 100% and a ring at 0% look alike at a glance and the gap gives the
 * eye somewhere to read the ends from.
 *
 * The arc is capped at the target. Beating it is said in words beside the
 * figure — an arc drawn to 180% wraps past its own start and reads as 80%,
 * which is the opposite of the news.
 */
export function gaugeBlock(b: ChartBlock): HTMLElement {
  const panel = el('section', { class: 'panel', part: 'panel' }, el('h3', {}, b.title))
  const g = b.gauge
  if (!g) {
    panel.append(el('p', { class: 'unaffected' }, 'Nothing to measure.'))
    return panel
  }

  // A 240° arc: 5/6 of the circumference, starting at the lower left.
  const r = 38
  const circumference = 2 * Math.PI * r
  const sweep = circumference * (240 / 360)

  const dial = svg('svg', {
    viewBox: '0 0 100 74', class: 'gauge', part: 'chart', 'aria-hidden': 'true',
  })
  const arc = (length: number, cls: string, colour: string) =>
    svg('circle', {
      cx: 50, cy: 50, r, fill: 'none', class: cls, stroke: colour,
      'stroke-width': 11, 'stroke-linecap': 'round',
      'stroke-dasharray': `${n(length)} ${n(circumference)}`,
      // 150° puts the arc's start at the lower left, so the gap is at the
      // bottom where the figure is not.
      transform: 'rotate(150 50 50)',
    })

  dial.append(arc(sweep, 'track', 'var(--cr-line)'))
  dial.append(arc(sweep * g.share, 'value', 'var(--cr-accent)'))
  panel.append(dial)

  panel.append(el('p', { class: 'stat gauge-value', part: 'stat' }, g.formatted))
  panel.append(el('p', { class: 'delta' },
    `${g.targetLabel} ${g.targetFormatted}`,
    ...(g.over ? [' · ', el('b', { class: 'up' }, g.over)] : [])))

  // The figures are the accessible reading of the arc, which is decorative.
  panel.setAttribute('aria-label',
    `${b.title}: ${g.formatted} of ${g.targetLabel} ${g.targetFormatted}`)
  return panel
}
