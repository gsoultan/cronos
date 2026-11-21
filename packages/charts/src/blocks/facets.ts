import type { ChartBlock } from '../types'
import { el } from '../dom'
import { chartPanel } from '../chart'
import { columnBlock } from './column'
import { lineBlock } from './line'

/** A small multiple's plot: tall enough for a shape and a scale, short
 *  enough that three of them sit where one chart would. */
const PANEL_HEIGHT = 120

/**
 * Small multiples: a small chart for each series, side by side, all on the
 * one scale the server sent — so a panel that reaches higher is larger, not
 * drawn against a different axis. Four regions tangled across one plot
 * become four shapes that can each be read, and still compared.
 *
 * Each panel is the chart it would have been with one series, drawn by the
 * same renderer at a smaller size; the grid is the only thing here.
 */
export function facetsBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const grid = el('div', { class: 'facets', part: 'facets' })
  for (const g of b.groups ?? []) {
    const one: ChartBlock = { ...b, title: g.label, groups: [g], facets: false }
    grid.append(b.chart === 'column'
      ? columnBlock(one, PANEL_HEIGHT)
      : lineBlock(one, b.chart === 'area', PANEL_HEIGHT))
  }
  panel.append(grid)
  return panel
}
