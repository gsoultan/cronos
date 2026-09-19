import type { ChartBlock } from '../types'
import { el } from '../dom'
import { withTips } from '../tip'
import { stepOf } from '../palette'

/**
 * A funnel.
 *
 * Divs, not SVG: a funnel is a stack of centred bands with a label beside each,
 * which is a grid — and the trapezoid sides a "proper" funnel draws encode
 * nothing the band's own width does not already say.
 *
 * The ordinal ramp, not eight identities. Swapping two stages changes what the
 * chart says, so the colour carries the order: a reader sees the sequence
 * rather than discovering there was one by reading top to bottom.
 */
export function funnelBlock(b: ChartBlock): HTMLElement {
  const panel = el('section', { class: 'panel wide', part: 'panel' }, el('h3', {}, b.title))
  const stages = b.stages ?? []
  if (stages.length === 0) {
    panel.append(el('p', { class: 'unaffected' }, 'No data in this period.'))
    return panel
  }

  const tips = withTips(panel)
  panel.append(el('div', { class: 'funnel' }, ...stages.map((s, i) => {
    const band = el('div', {
      class: 'band', part: 'stage',
      // A floor, so a stage almost nobody reached is still a visible band
      // rather than a gap that reads as a missing row.
      style: `width: ${Math.max(s.share * 100, 4)}%; background: var(--cr-step-${stepOf(i)})`,
    })
    tips.bind(band, s.label, s.formatted)

    return el('div', { class: 'stage' },
      el('div', { class: 'stage-head' },
        el('span', { class: 'name' }, s.label),
        el('span', { class: 'v' }, s.formatted)),
      el('div', { class: 'band-row' }, band),
      // The fall is the thing a funnel is read for, and it belongs between the
      // two stages it describes rather than in a tooltip nobody opens.
      s.drop ? el('span', { class: 'drop' }, s.drop) : el('span', { class: 'drop' }, ''))
  })))
  return panel
}
