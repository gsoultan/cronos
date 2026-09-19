import type { ChartBlock, Rect } from '../types'
import { el } from '../dom'
import { withTips } from '../tip'
import { slotOf, stepOf } from '../palette'

/**
 * A treemap, laid out by the server.
 *
 * Absolutely positioned divs over a box with a fixed aspect ratio. The
 * rectangles arrive as fractions of the unit square — see run.Rect for why the
 * squarify runs there — so this only scales them, and every viewer and the PDF
 * get the same layout rather than three attempts at the same algorithm.
 *
 * Labels are drawn only where they fit. A treemap's small rectangles are the
 * ones whose labels overlap into a smear, and a name half in and half out of
 * its own box is worse than a tooltip.
 */
export function treemapBlock(b: ChartBlock): HTMLElement {
  const panel = el('section', { class: 'panel wide', part: 'panel' }, el('h3', {}, b.title))
  const rects = b.rects ?? []
  if (rects.length === 0) {
    panel.append(el('p', { class: 'unaffected' }, 'Nothing to divide up.'))
    return panel
  }

  const tips = withTips(panel)
  const stage = el('div', { class: 'tree' })
  const nested = rects.some((r) => r.depth === 1)

  for (const r of rects) {
    // A group's frame is a label and a border; its leaves are drawn over it.
    // Drawing the frame filled would hide everything inside it.
    const box = el('div', {
      class: r.depth === 0 && nested ? 'tree-frame' : 'tree-cell',
      part: 'cell',
      style: `left:${r.x * 100}%;top:${r.y * 100}%;width:${r.w * 100}%;height:${r.h * 100}%`
        + (r.depth === 0 && nested ? '' : `;background: ${colour(r, nested)}`),
    })
    if (r.depth === 0 && nested) {
      box.append(el('span', { class: 'tree-group' }, r.label))
    } else {
      tips.bind(box, r.group ? `${r.group} · ${r.label}` : r.label, r.formatted)
      // Roughly the box a 11px label needs, as a fraction of the stage.
      if (r.w > 0.14 && r.h > 0.12) {
        box.append(el('span', { class: 'tree-label' },
          el('b', {}, r.label),
          el('span', {}, r.formatted)))
      }
    }
    stage.append(box)
  }

  panel.append(stage)
  return panel
}

/**
 * A nested treemap colours by group — identity, so leaves that belong together
 * look it. A flat one colours by rank from the ordinal ramp, because with one
 * level the only thing left to say is the order.
 */
function colour(r: Rect, nested: boolean): string {
  return nested ? `var(--cr-series-${slotOf(r.slot)})` : `var(--cr-step-${stepOf(r.slot)})`
}
