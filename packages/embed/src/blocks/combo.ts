import type { ChartBlock, Track } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'
import { plot, on, PLOT_W } from '../plot'
import { withTips } from '../tip'
import { slotOf } from '../palette'

const HEIGHT = 58

/**
 * Bars and lines together.
 *
 * The bars of every track share each bucket's width, so two bar tracks sit
 * side by side rather than one hiding the other; the lines run over the top,
 * through the middle of each bucket.
 *
 * One scale unless a measure asked to leave it. Where a second one does exist
 * it is drawn on the right and its ticks wear its own track's colour — the one
 * place in this package where a colour is spent on text, because a reader
 * facing two scales has no other way to tell which number belongs to which.
 */
export function comboBlock(b: ChartBlock): HTMLElement {
  const panel = el('section', { class: 'panel wide', part: 'panel' }, el('h3', {}, b.title))
  const tracks = b.tracks ?? []
  const y = b.yAxis
  const buckets = tracks[0]?.bars ?? []

  if (!y || buckets.length === 0) {
    panel.append(el('p', { class: 'unaffected' }, 'No data in this period.'))
    return panel
  }

  const p = plot({ min: 0, max: 1, ticks: ticks(buckets.map((x) => x.label)) }, y, HEIGHT, true)
  const tips = withTips(panel)
  const scaleFor = (t: Track) => (t.secondary && b.axis2 ? b.axis2 : y)

  const bars = tracks.filter((t) => t.draw === 'bar')
  bars.forEach((t, i) => {
    const slot = slotOf(t.slot)
    t.bars.forEach((bar, j) => {
      const a = scaleFor(t)
      const [, top] = p.at(0, on(a, bar.value))
      const [, base] = p.at(0, on(a, Math.max(a.min, 0)))
      // Each bar track gets its own lane inside the bucket, with a gap either
      // side. Two fills that touch read as one fill.
      const slice = (PLOT_W / buckets.length) * 0.8
      const width = slice / bars.length
      const x = (j + 0.5) * (PLOT_W / buckets.length) - slice / 2 + i * width

      const rect = svg('rect', {
        x: n(x + 0.5), y: n(Math.min(top, base)), width: n(Math.max(width - 1, 0.5)),
        height: n(Math.abs(base - top)), fill: `var(--cr-series-${slot})`,
        class: 'col', part: 'bar',
      })
      tips.bind(rect, `${t.label} · ${bar.label}`, bar.formatted)
      p.canvas.append(rect)
    })
  })

  for (const t of tracks.filter((x) => x.draw === 'line')) {
    const a = scaleFor(t)
    const d = t.bars
      .map((bar, j) => {
        const [, py] = p.at(0, on(a, bar.value))
        const px = (j + 0.5) * (PLOT_W / buckets.length)
        return `${j ? 'L' : 'M'}${n(px)} ${n(py)}`
      })
      .join('')
    p.canvas.append(svg('path', {
      d, class: 'line', part: 'line', stroke: `var(--cr-series-${slotOf(t.slot)})`,
    }))
  }

  panel.append(p.frame)
  if (b.axis2) panel.append(second(b.axis2, tracks))
  panel.append(key(tracks))
  return panel
}

/** The right-hand scale, when a measure opted out of the shared one. */
function second(axis: NonNullable<ChartBlock['axis2']>, tracks: Track[]): HTMLElement {
  const owner = tracks.find((t) => t.secondary)
  return el('p', { class: 'axis2' },
    el('i', { class: 'swatch', style: `background: var(--cr-series-${slotOf(owner?.slot ?? 1)})` }),
    `${owner?.label ?? 'Second axis'}: ${axis.ticks[0]?.label ?? ''}–${
      axis.ticks[axis.ticks.length - 1]?.label ?? ''}`)
}

/**
 * The legend, which a combo always has.
 *
 * Always, even at two measures, because a combo's whole premise is that the
 * marks mean different things — and the mark shape says bar-or-line, not which
 * measure. A track on its own scale says so here too: the reader otherwise has
 * no way to know that one of these lines is not comparable to the bars beside
 * it, which is the failure mode a second axis exists to cause.
 */
function key(tracks: Track[]): HTMLElement {
  return el('div', { class: 'legend', part: 'legend' },
    ...tracks.map((t) =>
      el('span', { class: 'key' },
        el('i', {
          class: t.draw === 'line' ? 'swatch rule-swatch' : 'swatch',
          style: `background: var(--cr-series-${slotOf(t.slot)})`,
        }),
        t.secondary ? `${t.label} (right)` : t.label)))
}

/** Bucket labels, thinned to what a panel's width holds. */
function ticks(labels: string[]) {
  const every = Math.max(1, Math.ceil(labels.length / 6))
  return labels
    .map((label, i) => ({ at: (i + 0.5) / labels.length, label, i }))
    .filter((t) => t.i % every === 0)
}
