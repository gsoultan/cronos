import { el } from './dom'
import type { LegendStop } from './types'
import { slotOf } from './palette'

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
 *
 * Given `toggle`, each key is a button that hides its series and shows it
 * again: the question a crowded chart is asked most is "without that one".
 */
export function legend(items: { label: string; slot: number; line?: boolean }[],
  toggle?: (i: number, on: boolean) => void): HTMLElement | null {
  if (items.length < 2) return null
  return el('div', { class: 'legend', part: 'legend' }, ...items.map((g, i) => {
    const swatch = el('i', {
      class: g.line ? 'swatch rule-swatch' : 'swatch',
      style: `background: var(--cr-series-${slotOf(g.slot)})`,
    })
    if (!toggle) return el('span', { class: 'key' }, swatch, g.label)
    const key = el('button', { type: 'button', class: 'key', 'aria-pressed': 'true' }, swatch, g.label)
    key.addEventListener('click', () => {
      const on = key.getAttribute('aria-pressed') !== 'true'
      key.setAttribute('aria-pressed', String(on))
      toggle(i, on)
    })
    return key
  }))
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
