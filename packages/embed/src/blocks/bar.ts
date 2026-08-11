import type { BarBlock } from '../types'
import { el } from '../dom'

/**
 * A horizontal bar chart, in divs.
 *
 * Not SVG: horizontal bars with a label column are a grid, and expressing a
 * grid in SVG means computing text widths in script — which costs more bytes
 * than the whole chart and gets it wrong once the host page's font loads late.
 *
 * Bars are drawn against the largest value rather than a zero-based axis
 * chosen by a scale library, because the label beside each bar carries the
 * real number. The bar ranks; the label states.
 */
export function barBlock(b: BarBlock): HTMLElement {
  const max = Math.max(...b.series.map((s) => s.value), 0)
  const rows = b.series.map((s) =>
    el('div', { class: 'bar-row' },
      el('span', {}, s.label),
      el('div', { class: 'track' },
        el('div', {
          class: 'fill',
          part: 'bar',
          // A zero-width bar reads as a missing row rather than a small one,
          // so the CSS keeps a 2px floor and this only sets the proportion.
          style: `width: ${max > 0 ? (s.value / max) * 100 : 0}%`,
        })),
      el('span', { class: 'v' }, s.formatted)))

  return el('section', { class: 'panel wide', part: 'panel' },
    el('h3', {}, b.title),
    el('div', { class: 'bars' }, ...rows))
}
