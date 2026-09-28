import type { Bar, ChartBlock, Group } from '../types'
import { svg, n } from '../svg'
import { cartesian, type Cartesian } from '../axes'
import { measure, sized, uid } from '../frame'
import { chartPanel, host, hue, nothing, PLOT_HEIGHT } from '../chart'
import { legend } from '../legend'
import { withTips, type Tips } from '../tip'

/** Past this many points a series is a line, not a line of dots. */
const MARKED = 24

/**
 * A line or area chart.
 *
 * Each point sits in the middle of its category's band, as a column would, so
 * the first and last labels have the room the others have rather than hanging
 * off the plot's edges. A dot marks each point while there are few enough to
 * count; past that the crosshair is what answers "what was March", for every
 * series at once.
 *
 * An area fades toward its floor, so a second series behind it still shows
 * through; stacked, each band is solid and sits on the ones below it.
 */
export function lineBlock(b: ChartBlock, area: boolean): HTMLElement {
  const panel = chartPanel(b.title)
  const y = b.yAxis
  const groups: Group[] = b.groups ?? [{ label: b.title, slot: 0, bars: b.series ?? [] }]
  const buckets = groups[0]?.bars ?? []
  if (!y || buckets.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  const shown = groups.map(() => true)
  let entered = false
  const draw = (width: number) => {
    const c = cartesian({ width, height: PLOT_HEIGHT, m: measure(at), y, categories: buckets.map((x) => x.label) })
    const xs = buckets.map((_, i) => c.band(i).x + c.band(i).w / 2)
    const floors = buckets.map(() => 0)
    const tops = groups.map((g, k) => {
      if (!shown[k]) return []
      const base = [...floors]
      const top = g.bars.map((bar, i) => (b.stacked ? (floors[i] = (floors[i] ?? 0) + bar.value) : bar.value))
      series(c, g, top.map((v, i) => [xs[i] ?? 0, c.y(v)]), area, b.stacked ? base : null, buckets.length,
        c.y(Math.max(y.min, 0)))
      return top
    })
    crosshair(c, xs, groups, tops, buckets, tips)
    if (!entered) c.svg.classList.add('enter')
    entered = true
    at.replaceChildren(c.svg)
  }
  const redraw = sized(at, draw)
  const key = legend(groups, (i, on) => { shown[i] = on; redraw() })
  if (key) panel.append(key)
  return panel
}

/** One series: its band or fade, its line, and its dots while they count. */
function series(c: Cartesian, g: Group, pts: [number, number][], area: boolean,
  floors: number[] | null, count: number, zero: number) {
  const colour = hue(g.slot)
  if (area) {
    // Down to the stack below, or to nothing — which is the axis' floor
    // unless the data runs below it.
    const floor = floors
      ? floors.map((v, i) => `L${n(pts[i]?.[0] ?? 0)} ${n(c.y(v))}`).reverse().join('')
      : `L${n(pts[pts.length - 1]?.[0] ?? 0)} ${n(zero)}L${n(pts[0]?.[0] ?? 0)} ${n(zero)}`
    const shade = svg('path', { class: floors ? 'area stacked' : 'area', part: 'area', d: `${trace(pts)}${floor}Z` })
    if (floors) shade.style.fill = colour
    else shade.style.fill = `url(#${fade(c, colour)})`
    c.marks.append(shade)
  }
  const line = svg('path', { class: 'line', part: 'line', d: trace(pts), pathLength: 1 })
  line.style.stroke = colour
  c.marks.append(line)
  if (count > MARKED) return
  for (const [x, y] of pts) {
    const dot = svg('circle', { class: 'point', cx: n(x), cy: n(y), r: 3.5 })
    dot.style.fill = colour
    c.marks.append(dot)
  }
}

/** A vertical fade in a series' colour, from the line down to nothing. */
function fade(c: Cartesian, colour: string): string {
  const id = uid('fade')
  const stop = (offset: number, opacity: number) => {
    const s = svg('stop', { offset })
    s.style.stopColor = colour
    s.style.stopOpacity = String(opacity)
    return s
  }
  c.svg.prepend(svg('defs', {}, svg('linearGradient', { id, x1: 0, y1: 0, x2: 0, y2: 1 }, stop(0, 0.28), stop(1, 0.02))))
  return id
}

function trace(points: [number, number][]): string {
  return points.map(([x, y], i) => `${i ? 'L' : 'M'}${n(x)} ${n(y)}`).join('')
}

/**
 * A vertical rule that follows the pointer to the nearest category, a dot on
 * every series there, and a tooltip reading all of them beside their colours.
 *
 * One target over the whole plot rather than one per point: a point on a line
 * has no area to hover, and the question a line chart is asked is "what was
 * every series then", not "what was this dot".
 */
function crosshair(c: Cartesian, xs: number[], groups: Group[], tops: number[][], buckets: Bar[], tips: Tips) {
  const { x, y, w, h } = c.plot
  const rule = svg('line', { class: 'rule', x1: 0, x2: 0, y1: y, y2: n(y + h), visibility: 'hidden' })
  const focus = svg('g', { class: 'focus' })
  const hit = svg('rect', { class: 'hit', x, y, width: n(w), height: n(h) })
  c.svg.append(rule, focus, hit)

  hit.addEventListener('pointermove', (e) => {
    const box = c.svg.getBoundingClientRect()
    const px = e.clientX - box.left
    let i = 0
    xs.forEach((at, k) => { if (Math.abs(at - px) < Math.abs((xs[i] ?? 0) - px)) i = k })
    const at = n(xs[i] ?? 0)
    rule.setAttribute('x1', at)
    rule.setAttribute('x2', at)
    rule.setAttribute('visibility', 'visible')
    focus.replaceChildren(...groups.flatMap((g, k) => {
      const v = tops[k]?.[i]
      if (v === undefined) return []
      const dot = svg('circle', { class: 'point on', cx: at, cy: n(c.y(v)), r: 5 })
      dot.style.fill = hue(g.slot)
      return [dot]
    }))
    const rows = groups.flatMap((g, k) => (tops[k]?.length
      ? [{ colour: hue(g.slot), label: g.label, value: g.bars[i]?.formatted ?? '—' }] : []))
    tips.show(e, buckets[i]?.label ?? '', rows.length === 1 ? rows[0]?.value : undefined,
      rows.length > 1 ? rows : undefined)
  })
  hit.addEventListener('pointerleave', () => {
    rule.setAttribute('visibility', 'hidden')
    focus.replaceChildren()
    tips.hide()
  })
}
