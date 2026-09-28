import type { ChartBlock } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'
import { arc, label, sizeToFit } from '../frame'
import { chartPanel, hue, nothing } from '../chart'
import { withTips } from '../tip'

const SIZE = 200
const R = 96

/**
 * A pie or a donut, and beside it every slice's name, value and share.
 *
 * Slices are paths, each hoverable as the slice it looks like, with a thin
 * line of surface between neighbours: two fills that touch read as one fill.
 * The one under the pointer moves out a little, and its row in the list
 * lights with it. A donut's middle carries the total, which is the number a
 * reader of a share chart checks first.
 *
 * A pie of negative values is not a pie. They are left out and the chart says
 * how many were, rather than dropping them where nobody can tell.
 */
export function pieBlock(b: ChartBlock, donut: boolean): HTMLElement {
  const panel = chartPanel(b.title, false)
  const all = b.series ?? []
  const series = all.filter((s) => s.value > 0)
  const total = series.reduce((sum, s) => sum + s.value, 0)
  if (total <= 0) return nothing(panel, 'Nothing to divide up.')

  const tips = withTips(panel)
  const dial = svg('svg', { viewBox: `0 0 ${SIZE} ${SIZE}`, class: 'pie enter', part: 'chart', 'aria-hidden': 'true' })
  const rows = el('ul', { class: 'slices' })
  const inner = donut ? R * 0.62 : 0
  const pad = series.length > 1 ? 0.012 : 0
  let from = 0

  series.forEach((s, i) => {
    const sweep = (s.value / total) * Math.PI * 2
    const mid = from + sweep / 2
    const share = `${Math.round((s.value / total) * 1000) / 10}%`
    const slice = svg('path', {
      class: 'slice', part: 'slice',
      d: arc(SIZE / 2, SIZE / 2, inner, R, from + pad / 2, from + Math.max(sweep - pad / 2, pad / 2 + 1e-4)),
    })
    slice.style.fill = hue(i)
    tips.bind(slice, s.label, `${s.formatted} · ${share}`)
    const row = el('li', {},
      el('i', { class: 'swatch', style: `background: ${hue(i)}` }),
      el('span', { class: 'name' }, s.label),
      el('span', { class: 'v' }, s.formatted),
      el('span', { class: 'share' }, share))
    // The slice and its row light together, from either end.
    const on = (lit: boolean) => {
      slice.style.transform = lit ? `translate(${n(Math.sin(mid) * 5)}px, ${n(-Math.cos(mid) * 5)}px)` : ''
      row.classList.toggle('on', lit)
    }
    for (const node of [slice, row]) {
      node.addEventListener('pointerenter', () => on(true))
      node.addEventListener('pointerleave', () => on(false))
    }
    dial.append(slice)
    rows.append(row)
    from += sweep
  })

  const middle = b.totals?.[0]
  if (donut && middle) {
    // Across the hole, whatever the figure's length: a total of millions
    // set at the size of one of thousands ran out over the ring.
    const figure = label(SIZE / 2, SIZE / 2 + 2, middle.formatted, 'centre')
    figure.style.fontSize = `${n(sizeToFit(middle.formatted, inner * 1.7, 22))}px`
    dial.append(figure, label(SIZE / 2, SIZE / 2 + 22, middle.label, 'centre-label'))
  }
  panel.append(el('div', { class: 'pie-wrap' }, dial, rows))
  const left = all.length - series.length
  if (left > 0) {
    panel.append(el('p', { class: 'unaffected' },
      `${left} ${left === 1 ? 'value' : 'values'} of nothing or less left out: a share cannot be less than none.`))
  }
  return panel
}
