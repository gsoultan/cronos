import type { TableBlock } from '../types'
import { el } from '../dom'

/**
 * Rows, scrolling in their own box.
 *
 * The scroller is the block, not the page: an embedded report sits inside
 * someone else's layout, and a wide table that widens the document breaks
 * their page rather than ours.
 */
export function tableBlock(b: TableBlock): HTMLElement {
  const head = el('tr', {}, ...b.columns.map((c) =>
    el('th', c.align === 'right' ? { class: 'r' } : {}, c.label)))

  const body = b.rows.map((row) =>
    el('tr', {}, ...row.map((cell, i) =>
      el('td', b.columns[i]?.align === 'right' ? { class: 'r' } : {}, cell))))

  const panel = el('section', { class: 'panel wide', part: 'panel' },
    el('h3', {}, b.title))

  if (b.total !== undefined && b.total > b.rows.length) {
    // Saying which is shown beats letting someone conclude the report is
    // wrong because they counted 50 of 1,284 invoices.
    panel.append(el('p', { class: 'unaffected' },
      `Showing ${b.rows.length} of ${b.total} rows`))
  }

  panel.append(el('div', { class: 'scroll' },
    el('table', { part: 'table' },
      el('thead', {}, head),
      el('tbody', {}, ...body))))

  return panel
}
