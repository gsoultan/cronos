import type { TableBlock, TableColumn } from '../types'
import { el } from '../dom'
import { svg } from '../svg'

/**
 * Rows, scrolling in their own box.
 *
 * The scroller is the block, not the page: an embedded report sits inside
 * someone else's layout, and a wide table that widens the document breaks
 * their page rather than ours.
 */
export function tableBlock(b: TableBlock): HTMLElement {
  const columns = b.columns ?? []
  const head = el('tr', {}, ...columns.map((c) =>
    el('th', c.align === 'right' ? { class: 'r' } : {}, c.label)))

  const body = (b.rows ?? []).map((row, r) =>
    el('tr', {}, ...row.map((cell, i) => cellOf(columns[i], r, cell))))

  const panel = el('section', { class: 'panel wide', part: 'panel' },
    el('h3', {}, b.title))

  if (b.total !== undefined && b.total > (b.rows?.length ?? 0)) {
    // Saying which is shown beats letting someone conclude the report is
    // wrong because they counted 50 of 1,284 invoices.
    panel.append(el('p', { class: 'unaffected' },
      `Showing ${b.rows?.length ?? 0} of ${b.total} rows`))
  }

  panel.append(el('div', { class: 'scroll' },
    el('table', { part: 'table' },
      el('thead', {}, head),
      el('tbody', {}, ...body))))

  return panel
}

const SPARK_W = 72
const SPARK_H = 18

/**
 * One cell, with what its column draws: a data bar behind the figure, or the
 * row's sparkline in place of one. The lengths and heights are the server's;
 * this only places them.
 */
export function cellOf(c: TableColumn | undefined, r: number, cell: string): HTMLElement {
  const align = c?.align === 'right' ? 'r' : ''
  const bar = c?.bars?.[r]
  if (bar) {
    return el('td', { class: `${align} barred` },
      el('span', { class: `databar${bar < 0 ? ' neg' : ''}`, style: `width: calc((100% - 8px) * ${Math.abs(bar)})` }),
      el('span', { class: 'figure' }, cell))
  }
  const line = c?.spark?.[r]
  if (line && line.length > 1) {
    return el('td', { class: 'sparked' }, sparkOf(line))
  }
  return el('td', align ? { class: align } : {}, cell)
}

function sparkOf(points: number[]): SVGSVGElement {
  const x = (i: number) => 2 + (i / (points.length - 1)) * (SPARK_W - 4)
  const y = (v: number) => 2 + (1 - v) * (SPARK_H - 4)
  const d = points.map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)} ${y(v).toFixed(1)}`).join('')
  const last = points.length - 1
  const root = svg('svg', { class: 'rowspark', width: SPARK_W, height: SPARK_H,
    viewBox: `0 0 ${SPARK_W} ${SPARK_H}`, 'aria-hidden': 'true' })
  root.append(
    svg('path', { class: 'trace', d }),
    svg('circle', { class: 'end', cx: x(last), cy: y(points[last] ?? 0.5), r: 2.25 }))
  return root
}
