import type { Cell, ChartBlock, LegendStop } from '../types'
import { svg, n } from '../svg'
import { canvas, fit, label, measure, sized, type Measure } from '../frame'
import { chartPanel, host, nothing } from '../chart'
import { rampLegend } from '../legend'
import { withTips, type Tips } from '../tip'
import { RAMP_STEPS } from '../palette'

const PX = 11

/**
 * A heatmap: two dimensions against a measure.
 *
 * Each cell carries its number where the number fits, in ink on the light
 * shades and in the surface colour on the dark ones, and the key under the
 * grid says what each shade spans. A heatmap without either is a pattern a
 * reader can see and not read.
 *
 * Every pair is present because the server fills the grid in; a cell nothing
 * matched is drawn as an outline rather than the lightest shade, because "no
 * rows" and "rows totalling nearly nothing" are different answers.
 */
export function heatmapBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const cells = b.cells ?? []
  const rows = b.heatRows ?? []
  const columns = b.heatColumns ?? []
  if (cells.length === 0 || columns.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  const byKey = new Map(cells.map((c) => [`${c.row}\u0000${c.column}`, c]))
  let entered = false
  sized(at, (width) => {
    const root = grid(width, rows, columns, byKey, measure(at), tips)
    if (!entered) root.classList.add('enter')
    entered = true
    at.replaceChildren(root)
  })
  const key = rampLegend(stops(cells))
  if (key) panel.append(key)
  return panel
}

function grid(width: number, rows: string[], columns: string[], byKey: Map<string, Cell>,
  m: Measure, tips: Tips): SVGSVGElement {
  const rowW = Math.min(Math.ceil(Math.max(0, ...rows.map((r) => m(r, PX)))) + 10, width * 0.3)
  const cw = Math.max(4, (width - rowW) / Math.max(columns.length, 1))
  const ch = Math.max(22, Math.min(40, cw * 0.62))
  const top = 20
  const root = canvas(width, top + rows.length * ch + 2)

  // Column heads thinned to every second or third where they would touch.
  const widest = Math.max(0, ...columns.map((c) => m(c, PX)))
  const every = Math.max(1, Math.ceil((Math.min(widest, 120) + 8) / cw))
  columns.forEach((c, j) => {
    if (j % every === 0) root.append(label(rowW + cw * (j + 0.5), 13, fit(c, cw * every - 6, PX, m), 'tick'))
  })
  rows.forEach((r, i) => {
    const y = top + i * ch
    root.append(label(0, y + ch / 2 + 4, fit(r, rowW - 10, PX, m), 'tick', 'start'))
    columns.forEach((c, j) => {
      const cell = byKey.get(`${r}\u0000${c}`)
      const x = rowW + j * cw
      const step = Math.min(cell?.step ?? 0, RAMP_STEPS - 1) + 1
      const box = svg('rect', {
        class: cell && !cell.empty ? `cell s${step}` : 'cell none', part: 'cell',
        x: n(x + 1), y: n(y + 1), width: n(Math.max(cw - 2, 1)), height: n(ch - 2), rx: 3,
      })
      if (cell && !cell.empty) box.style.fill = `var(--cr-ramp-${step})`
      tips.bind(box, `${r} · ${c}`, !cell || cell.empty ? 'No rows' : cell.formatted)
      root.append(box)
      if (cell && !cell.empty && m(cell.formatted, PX) <= cw - 8) {
        root.append(label(x + cw / 2, y + ch / 2 + 4, cell.formatted, `cell-value s${step}`))
      }
    })
  })
  return root
}

/** Anything drawn in a shade of the ramp: a heatmap's cell, a calendar's day. */
type Shaded = Pick<Cell, 'step' | 'value' | 'formatted'> & { empty?: boolean }

/**
 * What each shade spans, from the cells themselves: the lowest and highest
 * value drawn in it. A shade with no cell in it is left out of the key rather
 * than given a range nothing on the grid has.
 */
export function stops(cells: Shaded[]): LegendStop[] {
  const lo = new Map<number, Shaded>()
  const hi = new Map<number, Shaded>()
  for (const c of cells) {
    if (c.empty) continue
    const s = Math.min(c.step, RAMP_STEPS - 1)
    if (!lo.has(s) || c.value < (lo.get(s)?.value ?? 0)) lo.set(s, c)
    if (!hi.has(s) || c.value > (hi.get(s)?.value ?? 0)) hi.set(s, c)
  }
  return [...lo.keys()].sort((a, b) => a - b)
    .map((step) => ({ step, from: lo.get(step)?.formatted ?? '', to: hi.get(step)?.formatted ?? '' }))
}
