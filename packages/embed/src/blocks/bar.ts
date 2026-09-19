import type { Bar, ChartBlock, Group } from '../types'
import { el } from '../dom'
import { legend } from '../legend'
import { withTips, type Tips } from '../tip'

/**
 * A horizontal bar chart, in divs.
 *
 * Not SVG: horizontal bars with a label column are a grid, and expressing a
 * grid in SVG means computing text widths in script — which costs more bytes
 * than the whole chart and gets it wrong once the host page's font loads late.
 * That argument holds for the stacked and grouped forms too, which are the
 * same grid with more cells.
 *
 * Bars are drawn against the largest value rather than a zero-based axis
 * chosen by a scale library, because the label beside each bar carries the
 * real number. The bar ranks; the label states.
 */
export function barBlock(b: ChartBlock): HTMLElement {
  const panel = el('section', { class: 'panel wide', part: 'panel' }, el('h3', {}, b.title))
  const tip = withTips(panel)
  const groups = b.groups ?? []

  if (groups.length > 0) {
    const rows = b.stacked ? stacked(groups, b.totals ?? [], tip) : grouped(groups, tip)
    panel.append(el('div', { class: 'bars' }, ...rows))
    const key = legend(groups)
    if (key) panel.append(key)
    return panel
  }

  /* `?? []` because a nil slice from an older server arrives as a missing
     key, and the emptiest report is the one most likely to hit it. */
  const series = b.series ?? []
  const max = Math.max(...series.map((s) => s.value), 0)
  panel.append(el('div', { class: 'bars' }, ...series.map((s) =>
    el('div', { class: 'bar-row' },
      el('span', {}, s.label),
      el('div', { class: 'track' }, fill(s, max, 0, tip, s.label) ?? ''),
      el('span', { class: 'v' }, s.formatted)))))
  return panel
}

/**
 * One track per bucket, segments laid end to end.
 *
 * Every group covers every bucket because the server pads them, so a bucket's
 * segments can be read straight off the same index in each group without
 * reconciling which series were missing.
 */
function stacked(groups: Group[], totals: Bar[], tip: Tips): HTMLElement[] {
  const buckets = groups[0]?.bars ?? []
  const max = Math.max(...totals.map((t) => t.value), 0)

  return buckets.map((bucket, i) =>
    el('div', { class: 'bar-row' },
      el('span', {}, bucket.label),
      el('div', { class: 'track stack' },
        ...groups.map((g) => fill(g.bars[i], max, g.slot, tip, `${g.label} · ${bucket.label}`))
          // A series with nothing in this bucket contributes no segment at
          // all. A zero-width one would still show the 2px gap beside it,
          // which reads as a sliver of data that is not there.
          .filter((node): node is HTMLElement => node !== null)),
      el('span', { class: 'v' }, totals[i]?.formatted ?? '—')))
}

/** One mini-track per series, under a shared bucket label. */
function grouped(groups: Group[], tip: Tips): HTMLElement[] {
  const buckets = groups[0]?.bars ?? []
  const max = Math.max(...groups.flatMap((g) => g.bars.map((x) => x.value)), 0)

  return buckets.map((bucket, i) =>
    el('div', { class: 'bar-group' },
      el('span', { class: 'bucket' }, bucket.label),
      el('div', { class: 'series' }, ...groups.map((g) =>
        el('div', { class: 'bar-row thin' },
          el('div', { class: 'track' },
            fill(g.bars[i], max, g.slot, tip, `${g.label} · ${bucket.label}`) ?? ''),
          el('span', { class: 'v' }, g.bars[i]?.formatted ?? '—'))))))
}

/**
 * One drawn bar.
 *
 * A zero-width bar reads as a missing row rather than a small one, so the CSS
 * keeps a 2px floor and this only sets the proportion.
 */
function fill(bar: Bar | undefined, max: number, slot: number, tip: Tips,
  label: string): HTMLElement | null {
  if (!bar || bar.value === 0) return null
  const node = el('div', {
    class: 'fill',
    part: 'bar',
    style: `width: ${max > 0 ? (bar.value / max) * 100 : 0}%; background: var(--cr-series-${slot + 1})`,
  })
  tip.bind(node, label, bar.formatted)
  return node
}
