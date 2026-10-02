import type { Bar, StatBlock } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'
import { canvas, sized } from '../frame'
import { host, hue } from '../chart'
import { withTips, type Tips } from '../tip'

const SPARK = 36

/**
 * A single number, big, and how it moved.
 *
 * The change sits in a pill tinted by whether it is good news, and the pill
 * carries its own `good` flag rather than being coloured by direction.
 * Outstanding invoices rising is bad and revenue rising is good, and only the
 * engine that knows what the measure means can say which — a component that
 * colours by the arrow gets half of them backwards.
 *
 * With a trend, the number's periods run under it as a line: its shape, not
 * its figures, which the tile and its change already say — so no scale, the
 * latest period marked, and each period's figure a point away.
 */
export function statBlock(b: StatBlock): HTMLElement {
  // The figure is set as large as its tile has room for: the stylesheet
  // divides the tile's width by the figure's length, which is all it needs to
  // be told. At one size, a narrow tile cut the last digit off, and a number
  // missing a digit is a different number.
  const value = el('p', { class: 'stat', part: 'stat' }, b.value)
  value.style.setProperty('--n', String(Math.max(b.value.length, 1)))
  const panel = el('section', { class: 'panel', part: 'panel' },
    el('h3', {}, b.title), el('div', { class: 'stat-fit' }, value))

  if (b.delta) {
    const tone = b.delta.good ? 'up' : 'down'
    panel.append(el('p', { class: 'delta' },
      el('b', { class: `pill ${tone}` }, `${b.delta.dir === 'up' ? '▲' : '▼'} ${b.delta.value}`),
      b.delta.label ? ` ${b.delta.label}` : ''))
  }
  const trend = b.trend ?? []
  if (trend.length > 1) {
    const tips = withTips(panel)
    const at = host(panel)
    // Guessed at a tile's width, not a chart's: drawn first at 640 it
    // widened its tile's track until the real width came, and the tiles
    // beside it were resized twice in one frame — a loop WebKit reports.
    sized(at, (width) => at.replaceChildren(sparkline(width, trend, tips)), 220)
  }
  return panel
}

function sparkline(width: number, trend: Bar[], tips: Tips): SVGSVGElement {
  const values = trend.map((t) => t.value)
  const lo = Math.min(...values)
  const hi = Math.max(...values)
  const x = (i: number) => 3 + ((width - 6) * i) / (trend.length - 1)
  const y = (v: number) => SPARK - 4 - (SPARK - 8) * (hi === lo ? 0.5 : (v - lo) / (hi - lo))
  const d = trend.map((t, i) => `${i ? 'L' : 'M'}${n(x(i))} ${n(y(t.value))}`).join('')
  const root = canvas(width, SPARK, 'canvas spark')
  const area = svg('path', { class: 'area', d: `${d}L${n(x(trend.length - 1))} ${SPARK}L${n(x(0))} ${SPARK}Z` })
  const line = svg('path', { class: 'line', part: 'trend', d })
  area.style.fill = hue(0)
  line.style.stroke = hue(0)
  root.append(area, line)
  // A point a period, invisible until pointed at, so each one says its figure.
  trend.forEach((t, i) => {
    const last = i === trend.length - 1
    const dot = svg('circle', { class: last ? 'point' : 'point quiet', cx: n(x(i)), cy: n(y(t.value)), r: last ? 3.5 : 3 })
    dot.style.fill = hue(0)
    tips.bind(dot, t.label, t.formatted)
    root.append(dot)
  })
  return root
}
