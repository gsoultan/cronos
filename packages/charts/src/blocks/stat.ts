import type { StatBlock } from '../types'
import { el } from '../dom'

/**
 * A single number, big, and how it moved.
 *
 * The change sits in a pill tinted by whether it is good news, and the pill
 * carries its own `good` flag rather than being coloured by direction.
 * Outstanding invoices rising is bad and revenue rising is good, and only the
 * engine that knows what the measure means can say which — a component that
 * colours by the arrow gets half of them backwards.
 */
export function statBlock(b: StatBlock): HTMLElement {
  const panel = el('section', { class: 'panel', part: 'panel' },
    el('h3', {}, b.title),
    el('p', { class: 'stat', part: 'stat' }, b.value))

  if (b.delta) {
    const tone = b.delta.good ? 'up' : 'down'
    panel.append(el('p', { class: 'delta' },
      el('b', { class: `pill ${tone}` }, `${b.delta.dir === 'up' ? '▲' : '▼'} ${b.delta.value}`),
      b.delta.label ? ` ${b.delta.label}` : ''))
  }
  return panel
}
