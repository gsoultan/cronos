import type { ChartBlock } from '../types'
import { el } from '../dom'
import { withTips } from '../tip'
import { RAMP_STEPS } from '../palette'

/**
 * A heatmap: two dimensions against a measure.
 *
 * A CSS grid, not SVG. The cells are rectangles on a regular lattice with text
 * labels down two edges — which is what a grid is for, and doing it in SVG
 * would mean measuring label widths in script.
 *
 * Every pair is present because the server fills the grid in; a cell nothing
 * matched is drawn as absence rather than as the lightest shade, because "no
 * rows" and "rows totalling nearly nothing" are different answers and the
 * ramp's light end cannot say both.
 */
export function heatmapBlock(b: ChartBlock): HTMLElement {
  const panel = el('section', { class: 'panel wide', part: 'panel' }, el('h3', {}, b.title))
  const cells = b.cells ?? []
  const rows = b.heatRows ?? []
  const columns = b.heatColumns ?? []

  if (cells.length === 0 || columns.length === 0) {
    panel.append(el('p', { class: 'unaffected' }, 'No data in this period.'))
    return panel
  }

  const tips = withTips(panel)
  const grid = el('div', {
    class: 'heat-grid',
    // One column for the row labels, then one per column of data.
    style: `grid-template-columns: auto repeat(${columns.length}, minmax(0, 1fr))`,
  })

  // The corner, then the column headings.
  grid.append(el('span', { class: 'corner' }))
  for (const c of columns) grid.append(el('span', { class: 'col-head' }, c))

  for (const r of rows) {
    grid.append(el('span', { class: 'row-head' }, r))
    for (const c of columns) {
      const cell = cells.find((x) => x.row === r && x.column === c)
      const box = el('div', {
        class: cell?.empty ? 'cell none' : 'cell',
        part: 'cell',
        style: cell?.empty
          ? ''
          : `background: var(--cr-ramp-${Math.min(cell?.step ?? 0, RAMP_STEPS - 1) + 1})`,
      })
      tips.bind(box, `${r} · ${c}`, cell?.empty ? 'No rows' : (cell?.formatted ?? ''))
      grid.append(box)
    }
  }

  panel.append(grid)
  return panel
}
