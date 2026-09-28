import type { Axis, ChartBlock, Group } from '../types'
import { svg, n } from '../svg'
import { canvas, fit, label, measure, sized, type Measure } from '../frame'
import { chartPanel, host, hue, nothing } from '../chart'
import { legend } from '../legend'
import { withTips, type Tips } from '../tip'

const LABEL_PX = 11
const TICK_PX = 10

/**
 * A radar: each category a spoke around a circle, each series a closed shape
 * through its values along them, read against rings at the scale's ticks.
 *
 * It is for a profile — a few series across a handful of measures of the same
 * kind — rather than a trend or a ranking, which a line or a bar says better.
 * The rings are polygons rather than circles, because a value between two
 * spokes is on the straight edge joining them, not on an arc. The spokes'
 * names sit outside the web, anchored away from it, each cut to the room
 * between where it starts and the edge it runs towards.
 */
export function radarBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const groups: Group[] = b.groups ?? [{ label: b.title, slot: 0, bars: b.series ?? [] }]
  const spokes = groups[0]?.bars ?? []
  const y = b.yAxis
  if (!y || spokes.length === 0) return nothing(panel)
  if (spokes.length < 3) return nothing(panel, 'A radar needs three categories or more; a bar chart says two better.')

  const tips = withTips(panel)
  const at = host(panel)
  const shown = groups.map(() => true)
  let entered = false
  const redraw = sized(at, (width) => {
    const root = draw(width, y, groups, shown, measure(at), tips)
    if (!entered) root.classList.add('enter')
    entered = true
    at.replaceChildren(root)
  })
  const key = legend(groups, (i, on) => { shown[i] = on; redraw() })
  if (key) panel.append(key)
  return panel
}

function draw(width: number, y: Axis, groups: Group[], shown: boolean[], m: Measure, tips: Tips): SVGSVGElement {
  const spokes = groups[0]?.bars ?? []
  // Room for the names either side, and the web in what is left, no larger
  // than a panel's height would carry.
  const side = Math.min(Math.max(...spokes.map((s) => m(s.label, LABEL_PX))) + 12, width * 0.3)
  const r = Math.max(40, Math.min((width - side * 2) / 2, 150))
  const height = Math.round(r * 2 + 44)
  const cx = width / 2
  const cy = height / 2
  const angle = (i: number) => -Math.PI / 2 + (i * Math.PI * 2) / spokes.length
  const at = (i: number, share: number) =>
    [cx + Math.cos(angle(i)) * r * share, cy + Math.sin(angle(i)) * r * share] as const
  const share = (v: number) => Math.max(0, Math.min(1, y.max === y.min ? 0 : (v - y.min) / (y.max - y.min)))

  const root = canvas(width, height)
  const web = svg('g', { class: 'web' })
  const marks = svg('g', { class: 'marks' })
  root.append(web, marks)
  for (const t of y.ticks) {
    if (t.at <= 0) continue
    web.append(svg('path', { class: 'gridline web-ring', d: ring(spokes.length, (i) => at(i, t.at)) }))
    // Inside the ring, under its top, where the first spoke's name is not.
    web.append(label(cx + 4, cy - r * t.at + 11, t.label, 'tick', 'start'))
  }
  spokes.forEach((s, i) => {
    const [x, yy] = at(i, 1)
    web.append(svg('line', { class: 'gridline', x1: n(cx), y1: n(cy), x2: n(x), y2: n(yy) }))
    name(web, s.label, angle(i), cx + Math.cos(angle(i)) * (r + 8), cy + Math.sin(angle(i)) * (r + 8), width, m)
  })
  groups.forEach((g, k) => {
    if (!shown[k]) return
    shape(marks, g, groups.length > 1, (i) => at(i, share(g.bars[i]?.value ?? y.min)), tips)
  })
  return root
}

/** A closed path through one point per spoke. */
function ring(count: number, point: (i: number) => readonly [number, number]): string {
  let d = ''
  for (let i = 0; i < count; i++) {
    const [x, y] = point(i)
    d += `${i ? 'L' : 'M'}${n(x)} ${n(y)}`
  }
  return `${d}Z`
}

/** A spoke's name outside the web, running away from it on whichever side,
 *  in the room between where it starts and the edge it runs towards. */
function name(into: SVGGElement, text: string, a: number, x: number, y: number, width: number, m: Measure) {
  const cos = Math.cos(a)
  const anchor = Math.abs(cos) < 0.2 ? 'middle' : cos > 0 ? 'start' : 'end'
  const room = anchor === 'start' ? width - x - 2 : anchor === 'end' ? x - 2 : Math.min(x, width - x) * 2 - 4
  // Above the web the name sits on its point; below, it hangs from it.
  const dy = Math.sin(a) > 0.2 ? TICK_PX + 2 : Math.sin(a) < -0.2 ? -2 : 4
  into.append(label(x, y + dy, fit(text, room, LABEL_PX, m), 'name', anchor))
}

/** One series: its shape, washed, and a dot on each spoke to hover. */
function shape(into: SVGGElement, g: Group, named: boolean, point: (i: number) => readonly [number, number], tips: Tips) {
  const path = svg('path', { class: 'area radar', d: ring(g.bars.length, point) })
  path.style.fill = hue(g.slot)
  path.style.stroke = hue(g.slot)
  into.append(path)
  g.bars.forEach((v, i) => {
    const [x, y] = point(i)
    const dot = svg('circle', { class: 'point', part: 'point', cx: n(x), cy: n(y), r: 3.5 })
    dot.style.fill = hue(g.slot)
    tips.bind(dot, named ? `${g.label} · ${v.label}` : v.label, v.formatted)
    into.append(dot)
  })
}
