import type { TextBlock } from '../types'
import { el } from '../dom'

/**
 * Prose on a report — a heading, a note, a caveat beside a number.
 *
 * Neither renderer drew one. The server has emitted `kind: "text"` since the
 * format had four block kinds, and both viewers fell through to "this block
 * needs a newer viewer" for a sentence the author had written by hand. It read
 * as a version problem and was a missing case.
 */
export function textBlock(b: TextBlock): HTMLElement {
  const panel = el('section', { class: 'panel wide', part: 'panel' })
  if (b.title) panel.append(el('h3', {}, b.title))
  // textContent, never innerHTML: this is author-written text, and the rule
  // that there is no escaping to get wrong holds for every string here.
  panel.append(el('p', { class: 'prose' }, b.value ?? ''))
  return panel
}
