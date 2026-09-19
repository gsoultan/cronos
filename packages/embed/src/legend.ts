import { el } from './dom'
import type { Group, LegendStop } from './types'

/**
 * The key for a chart with more than one series.
 *
 * Always present past one series, because colour is the only thing telling two
 * series apart and a colour with no name is a question. One series gets none —
 * the title already names it, and a legend of one is furniture.
 *
 * The label wears ink, never the series colour: coloured text at 12px fails
 * contrast on half the palette, and the swatch beside it is already carrying
 * the identity.
 */
export function legend(groups: Group[]): HTMLElement | null {
  if (groups.length < 2) return null
  return el('div', { class: 'legend', part: 'legend' },
    ...groups.map((g) =>
      el('span', { class: 'key' },
        el('i', { class: 'swatch', style: `background: var(--cr-series-${g.slot + 1})` }),
        g.label)))
}

/**
 * The key for a map's sequential ramp.
 *
 * Bands rather than a gradient bar. The ramp is six steps because a reader
 * answers "same band or not" far better than "darker or not", and a legend
 * that shows a smooth gradient promises a precision the shading does not have.
 */
export function rampLegend(stops: LegendStop[]): HTMLElement | null {
  if (stops.length === 0) return null
  return el('div', { class: 'legend ramp', part: 'legend' },
    ...stops.map((s) =>
      el('span', { class: 'key' },
        el('i', { class: 'swatch', style: `background: var(--cr-ramp-${s.step + 1})` }),
        s.from === s.to ? s.from : `${s.from}–${s.to}`)))
}
