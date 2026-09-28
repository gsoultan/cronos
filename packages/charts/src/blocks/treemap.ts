import type { ChartBlock, Rect } from '../types'
import { svg, n } from '../svg'
import { canvas, fit, label, measure, sized } from '../frame'
import { chartPanel, host, nothing } from '../chart'
import { withTips } from '../tip'
import { slotOf, stepOf } from '../palette'

const HEAD = 18

/**
 * A treemap, laid out by the server.
 *
 * The rectangles arrive as fractions of the unit square — see run.Rect for why
 * the squarify runs there — so every viewer and the PDF get the same layout
 * rather than three attempts at one algorithm. This scales them, and gives
 * each group a strip at its top for its name: the leaves used to be laid over
 * the whole of their group, name and all, so a nested treemap never said
 * which group was which. The leaves inside give up that strip's height
 * between them, evenly, which keeps their areas in proportion to each other.
 *
 * A leaf is named where the name fits and cut to its width where it nearly
 * does; the whole of it is in the tooltip.
 */
export function treemapBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const rects = b.rects ?? []
  if (rects.length === 0) return nothing(panel, 'Nothing to divide up.')

  const tips = withTips(panel)
  const at = host(panel)
  const nested = rects.some((r) => r.depth === 1)
  const frames = new Map(rects.filter((r) => nested && r.depth === 0).map((r) => [r.label, r]))
  let entered = false
  sized(at, (width) => {
    const m = measure(at)
    const height = Math.round(Math.min(width * 0.62, 340))
    const root = canvas(width, height)
    const px = (r: Rect) => ({ x: r.x * width, y: r.y * height, w: r.w * width, h: r.h * height })

    for (const f of frames.values()) {
      const { x, y, w, h } = px(f)
      root.append(svg('rect', { class: 'tree-frame', x: n(x + 0.5), y: n(y + 0.5), width: n(w - 1), height: n(h - 1), rx: 4 }))
      if (h >= HEAD * 2 && w > 30) root.append(label(x + 6, y + 13, fit(f.label, w - 12, 10, m), 'tree-group', 'start'))
    }
    for (const r of rects) {
      if (nested && r.depth === 0) continue
      let { x, y, w, h } = px(r)
      const f = r.group ? frames.get(r.group) : undefined
      if (f) {
        const g = px(f)
        const head = g.h >= HEAD * 2 ? HEAD : 0
        const k = (g.h - head - 2) / g.h
        y = g.y + head + (y - g.y) * k
        h *= k
      }
      const cell = svg('rect', { class: 'tree-cell', part: 'cell', x: n(x + 1), y: n(y + 1), width: n(Math.max(w - 2, 0)), height: n(Math.max(h - 2, 0)), rx: 3 })
      cell.style.fill = nested ? `var(--cr-series-${slotOf(r.slot)})` : `var(--cr-step-${stepOf(r.slot)})`
      tips.bind(cell, r.group ? `${r.group} · ${r.label}` : r.label, r.formatted)
      root.append(cell)
      if (w >= 44 && h >= 34) {
        const name = fit(r.label, w - 12, 12, m)
        if (name) root.append(label(x + 7, y + 17, name, 'tree-label', 'start'),
          label(x + 7, y + 31, fit(r.formatted, w - 12, 11, m), 'tree-value', 'start'))
      }
    }
    if (!entered) root.classList.add('enter')
    entered = true
    at.replaceChildren(root)
  })
  return panel
}
