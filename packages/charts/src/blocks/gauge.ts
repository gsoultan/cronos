import type { ChartBlock } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'
import { label, sizeToFit } from '../frame'
import { chartPanel, nothing } from '../chart'

// The arc's ends sit at CY + R/2, and its stroke half as wide again below
// them; the end labels go under that, inside the box.
const W = 240
const H = 180
const CX = 120
const CY = 106
const R = 86
/** Two thirds of a turn, open at the bottom where the figures are. */
const SWEEP = (Math.PI * 4) / 3

/**
 * One number against a target.
 *
 * A stat tile answers "what is it"; this answers "is it enough", and the
 * target is the whole difference — so the dial is scaled from nothing to the
 * target, both ends are labelled, and the figure in the middle says how far
 * along it the value is. An open arc rather than a full ring, because a ring
 * at 100% and at 0% look alike at a glance.
 *
 * The arc stops at the target. Beating it turns the arc to the good colour
 * and says so in words — an arc drawn to 180% wraps past its own start and
 * reads as 80%, the opposite of the news. And nothing is drawn for nothing: a
 * rounded arc of no length is a dot at the start that reads as a small value.
 */
export function gaugeBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title, false)
  const g = b.gauge
  if (!g) return nothing(panel, 'Nothing to measure.')

  const start = -SWEEP / 2
  const dial = svg('svg', { viewBox: `0 0 ${W} ${H}`, class: 'gauge enter', part: 'chart', 'aria-hidden': 'true' })
  dial.append(svg('path', { class: 'track', d: stroke(start, start + SWEEP) }))
  if (g.share > 0) {
    const value = svg('path', { class: g.over ? 'reading over' : 'reading', d: stroke(start, start + SWEEP * Math.min(g.share, 1)), pathLength: 1 })
    dial.append(value)
  }
  const percent = g.target > 0 ? `${Math.round((g.value / g.target) * 1000) / 10}%` : ''
  const figure = label(CX, CY - 4, g.formatted, 'centre')
  figure.style.fontSize = `${n(sizeToFit(g.formatted, (R - 12) * 1.7, 26))}px`
  dial.append(
    figure,
    label(CX, CY + 20, percent ? `${percent} of ${g.targetLabel.toLowerCase()}` : '', 'centre-label'),
    label(end(start)[0], H - 4, '0', 'tick'),
    label(end(start + SWEEP)[0], H - 4, g.targetFormatted, 'tick'))
  panel.append(dial)

  panel.append(el('p', { class: 'delta gauge-note' },
    `${g.targetLabel} ${g.targetFormatted}`,
    ...(g.over ? [' · ', el('b', { class: 'up' }, g.over)] : [])))
  // The figures are the accessible reading of the dial, which is decorative.
  panel.setAttribute('aria-label', `${b.title}: ${g.formatted} of ${g.targetLabel} ${g.targetFormatted}`)
  return panel
}

/** A point on the dial at angle a, clockwise from twelve o'clock. */
function end(a: number): [number, number] {
  return [CX + R * Math.sin(a), CY - R * Math.cos(a)]
}

/** The dial's arc from a0 to a1, as a stroke. */
function stroke(a0: number, a1: number): string {
  const [x0, y0] = end(a0)
  const [x1, y1] = end(a1)
  return `M${n(x0)} ${n(y0)}A${R} ${R} 0 ${a1 - a0 > Math.PI ? 1 : 0} 1 ${n(x1)} ${n(y1)}`
}
