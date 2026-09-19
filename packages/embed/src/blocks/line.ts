import type { Bar, ChartBlock, Group } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'
import { plot, on, PLOT_W } from '../plot'
import { legend } from '../legend'
import { withTips } from '../tip'

const HEIGHT = 58

/**
 * A line or area chart.
 *
 * SVG here, where the bar chart is divs: a line is a single path through every
 * point, which is the one shape a grid of elements cannot express. It stretches
 * to the panel with `preserveAspectRatio: none`, and the strokes carry
 * `vector-effect: non-scaling-stroke` so a 2px line stays 2px at every width
 * rather than becoming a wedge.
 *
 * No marker per point. A dot on every month of a two-year series is forty-eight
 * dots and no information; the crosshair on hover is what answers "what was
 * March", and it answers it for every series at once.
 */
export function lineBlock(b: ChartBlock, area: boolean): HTMLElement {
  const panel = el('section', { class: 'panel wide', part: 'panel' }, el('h3', {}, b.title))
  const y = b.yAxis
  const groups: Group[] = b.groups ?? [{ label: b.title, slot: 0, bars: b.series ?? [] }]
  const buckets = groups[0]?.bars ?? []

  if (!y || buckets.length === 0) {
    panel.append(el('p', { class: 'unaffected' }, 'No data in this period.'))
    return panel
  }

  const p = plot({ min: 0, max: 1, ticks: xTicks(buckets) }, y, HEIGHT, true)
  // Stacked areas are drawn from the top down, so each band sits on the sum of
  // the ones below it rather than on the baseline.
  const floors = buckets.map(() => 0)

  for (const g of groups) {
    // Stacked tops accumulate into floors, so each band's top edge is the
    // running total and its base is where the previous band left off.
    const tops = g.bars.map((bar, i) => {
      if (!b.stacked) return bar.value
      return (floors[i] = (floors[i] ?? 0) + bar.value)
    })
    const points = tops.map((v, i) => p.at(xOf(i, buckets.length), on(y, v)))

    if (area) {
      const base = b.stacked
        ? tops.map((v, i) => p.at(xOf(i, buckets.length), on(y, v - (g.bars[i]?.value ?? 0))))
        : []
      p.canvas.append(svg('path', {
        class: 'area', part: 'area',
        fill: `var(--cr-series-${g.slot + 1})`,
        d: `${trace(points)} ${close(base, p, y, buckets.length)}`,
      }))
    }
    p.canvas.append(svg('path', {
      class: 'line', part: 'line',
      stroke: `var(--cr-series-${g.slot + 1})`,
      d: trace(points),
    }))
  }

  panel.append(p.frame)
  crosshair(panel, p.canvas, groups, buckets)
  const key = legend(groups)
  if (key) panel.append(key)
  return panel
}

/** Where bucket i sits, 0..1. A lone bucket goes in the middle rather than on
 *  the left edge, where it would read as the start of a series that is
 *  missing. */
function xOf(i: number, count: number): number {
  return count === 1 ? 0.5 : i / (count - 1)
}

function trace(points: [number, number][]): string {
  return points.map(([x, y], i) => `${i ? 'L' : 'M'}${n(x)} ${n(y)}`).join('')
}

/**
 * Seals an area band: back along the band below it when stacked, or along the
 * baseline when not.
 */
function close(base: [number, number][], p: ReturnType<typeof plot>, y: NonNullable<ChartBlock['yAxis']>,
  count: number): string {
  if (base.length > 0) {
    return `${base.reverse().map(([x, v]) => `L${n(x)} ${n(v)}`).join('')}Z`
  }
  const [, floor] = p.at(0, on(y, Math.max(y.min, 0)))
  return `L${n(PLOT_W * xOf(count - 1, count))} ${n(floor)}L${n(PLOT_W * xOf(0, count))} ${n(floor)}Z`
}

/** Evenly spaced bucket labels, thinned to what will fit. */
function xTicks(buckets: Bar[]) {
  // Every label on a 24-month series overlaps into an unreadable smear. Six is
  // about what a panel's width holds, and showing a sixth of them beats
  // showing all of them illegibly.
  const every = Math.max(1, Math.ceil(buckets.length / 6))
  return buckets
    .map((bar, i) => ({ at: xOf(i, buckets.length), label: bar.label, i }))
    .filter((t) => t.i % every === 0)
}

/**
 * A vertical rule that follows the pointer and reads every series at that
 * bucket at once.
 *
 * One listener on the canvas rather than a hit target per point: a point on a
 * line has no area to hover, and giving each one an invisible 20px circle is
 * both more nodes and a worse target than the whole column.
 */
function crosshair(panel: HTMLElement, canvas: SVGSVGElement, groups: Group[], buckets: Bar[]) {
  const tip = withTips(panel)
  const rule = svg('line', { class: 'rule', y1: 0, y2: HEIGHT, x1: 0, x2: 0, hidden: '' })
  canvas.append(rule)

  const hit = svg('rect', {
    x: 0, y: 0, width: PLOT_W, height: HEIGHT, fill: 'transparent', class: 'hit',
  })
  canvas.append(hit)

  hit.addEventListener('pointermove', (e) => {
    const box = canvas.getBoundingClientRect()
    const frac = (e.clientX - box.left) / box.width
    const i = Math.max(0, Math.min(buckets.length - 1, Math.round(frac * (buckets.length - 1))))
    const x = PLOT_W * xOf(i, buckets.length)
    rule.setAttribute('x1', n(x))
    rule.setAttribute('x2', n(x))
    rule.removeAttribute('hidden')

    // Every series at that bucket, which is the question a line chart is
    // actually asked — and the reason this is one crosshair rather than a hit
    // target per point.
    const all = groups.map((g) => `${g.label}: ${g.bars[i]?.formatted ?? '—'}`).join(' · ')
    tip.show(e, buckets[i]?.label ?? '', groups.length > 1 ? all : groups[0]?.bars[i]?.formatted)
  })
  hit.addEventListener('pointerleave', () => {
    rule.setAttribute('hidden', '')
    tip.hide()
  })
}
