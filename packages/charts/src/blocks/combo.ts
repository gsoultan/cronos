import type { Axis, ChartBlock, Track } from '../types'
import { svg, n } from '../svg'
import { cartesian, type Cartesian } from '../axes'
import { bar, measure, sized } from '../frame'
import { chartPanel, host, hue, nothing, PLOT_HEIGHT } from '../chart'
import { legend } from '../legend'
import { withTips, type Tips } from '../tip'

/**
 * Bars and lines together.
 *
 * The bars of every track share each category's band, side by side, so one
 * never hides another; the lines run over them through the middle of each
 * band, a dot on each point. A measure that asked for its own scale is read
 * against a second axis on the right, its numbers in its own colour — the one
 * place a colour is spent on text, because a reader facing two scales has no
 * other way to tell which number belongs to which. That axis used to be a
 * sentence under the chart saying its range, and the line it scaled was drawn
 * against nothing a reader could see.
 */
export function comboBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const tracks = b.tracks ?? []
  const y = b.yAxis
  const buckets = tracks[0]?.bars ?? []
  if (!y || buckets.length === 0) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  const shown = tracks.map(() => true)
  const second = tracks.find((t) => t.secondary)
  let entered = false
  const redraw = sized(at, (width) => {
    const c = cartesian({
      width, height: PLOT_HEIGHT, m: measure(at), y, categories: buckets.map((x) => x.label),
      y2: b.axis2, y2Colour: second ? hue(second.slot) : undefined,
    })
    const scale = (t: Track) => (t.secondary && b.axis2 ? b.axis2 : y)
    columns(c, tracks.filter((t, i) => shown[i] && t.draw === 'bar'), scale, tips)
    tracks.forEach((t, i) => { if (shown[i] && t.draw === 'line') line(c, t, scale(t), tips) })
    if (!entered) c.svg.classList.add('enter')
    entered = true
    at.replaceChildren(c.svg)
  })
  // Always a key, even at two measures: a combo's premise is that its marks
  // mean different things, and the shape says bar or line, not which measure.
  const key = legend(tracks.map((t) => ({
    label: t.secondary ? `${t.label} (right)` : t.label, slot: t.slot, line: t.draw === 'line',
  })), (i, on) => { shown[i] = on; redraw() })
  if (key) panel.append(key)
  return panel
}

/** Each bar track in a lane of its own inside every band. */
function columns(c: Cartesian, bars: Track[], scale: (t: Track) => Axis, tips: Tips) {
  bars.forEach((t, k) => {
    const a = scale(t)
    const zero = c.y(Math.max(a.min, 0), a)
    t.bars.forEach((v, i) => {
      const band = c.band(i)
      // Capped: two categories across a dashboard made columns a third of the
      // screen wide, which reads as blocks of colour rather than heights.
      const lane = Math.min((band.w * 0.72) / bars.length, 56)
      const left = band.x + (band.w - lane * bars.length) / 2 + k * lane
      const top = c.y(v.value, a)
      const path = svg('path', {
        class: 'col', part: 'bar',
        d: bar(left + 1, Math.min(top, zero), Math.max(lane - 2, 1), Math.max(Math.abs(zero - top), 1),
          v.value < 0 ? 'down' : 'up'),
      })
      path.style.fill = hue(t.slot)
      tips.bind(path, `${t.label} · ${v.label}`, v.formatted)
      c.marks.append(path)
    })
  })
}

/** A line through the middle of each band, a dot on each point. */
function line(c: Cartesian, t: Track, a: Axis, tips: Tips) {
  const pts = t.bars.map((v, i) => [c.band(i).x + c.band(i).w / 2, c.y(v.value, a)] as const)
  const path = svg('path', {
    class: 'line', part: 'line', pathLength: 1,
    d: pts.map(([x, y], i) => `${i ? 'L' : 'M'}${n(x)} ${n(y)}`).join(''),
  })
  path.style.stroke = hue(t.slot)
  c.marks.append(path)
  pts.forEach(([x, y], i) => {
    const dot = svg('circle', { class: 'point', cx: n(x), cy: n(y), r: 4 })
    dot.style.fill = hue(t.slot)
    const v = t.bars[i]
    if (v) tips.bind(dot, `${t.label} · ${v.label}`, v.formatted)
    c.marks.append(dot)
  })
}
