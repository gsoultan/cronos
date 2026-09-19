import type { ChartBlock } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'
import { withTips } from '../tip'
import { PALETTE_SIZE } from '../palette'

/**
 * A pie or a donut.
 *
 * Drawn as dashes on a circle's stroke rather than as a path per slice, which
 * is the whole reason both shapes are one file: a slice is a length of
 * stroke-dasharray, and the only difference between a pie and a donut is how
 * wide the stroke is and what radius it sits at. The arc-flag arithmetic an
 * `A` command needs — and the 180° case it gets wrong — never comes up.
 *
 * The gap between slices is the panel showing through, for the same reason a
 * stacked bar has one: two fills that touch read as one fill.
 */
export function pieBlock(b: ChartBlock, donut: boolean): HTMLElement {
  const panel = el('section', { class: 'panel', part: 'panel' }, el('h3', {}, b.title))
  const tips = withTips(panel)

  const series = (b.series ?? []).filter((s) => s.value > 0)
  const total = series.reduce((sum, s) => sum + s.value, 0)
  if (total <= 0) {
    panel.append(el('p', { class: 'unaffected' }, 'Nothing to divide up.'))
    return panel
  }

  // r and width together decide where the ring's inner and outer edges land.
  // A pie is the degenerate donut whose hole has no radius.
  const r = donut ? 34 : 25
  const width = donut ? 22 : 50
  const circumference = 2 * Math.PI * r
  // In user units, so it is the same visual gap whatever the panel's size —
  // the SVG scales uniformly.
  const gap = 1.2

  const ring = svg('svg', {
    viewBox: '0 0 100 100', class: 'pie', part: 'chart', 'aria-hidden': 'true',
  })
  let offset = 0
  series.forEach((s, i) => {
    const length = (s.value / total) * circumference
    const arc = svg('circle', {
      cx: 50, cy: 50, r, fill: 'none',
      stroke: `var(--cr-series-${(i % PALETTE_SIZE) + 1})`,
      'stroke-width': width,
      // The gap is taken off the drawn length rather than added to the
      // offset, so the slices still add up to the whole circle.
      'stroke-dasharray': `${n(Math.max(0, length - gap))} ${n(circumference)}`,
      'stroke-dashoffset': n(-offset),
      // A dash starts at three o'clock; a pie starts at twelve.
      transform: 'rotate(-90 50 50)',
    })
    tips.bind(arc, s.label, `${s.formatted} · ${Math.round((s.value / total) * 100)}%`)
    ring.append(arc)
    offset += length
  })

  panel.append(ring)
  // Direct labels rather than a legend box. A pie has at most a handful of
  // slices and the reader's question is "which is which and how much" — a
  // legend answers half of that and moves the other half across the panel.
  panel.append(el('ul', { class: 'slices' }, ...series.map((s, i) =>
    el('li', {},
      el('i', { class: 'swatch', style: `background: var(--cr-series-${(i % PALETTE_SIZE) + 1})` }),
      el('span', { class: 'name' }, s.label),
      el('span', { class: 'v' }, s.formatted)))))
  return panel
}
