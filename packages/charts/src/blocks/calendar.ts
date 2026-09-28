import type { ChartBlock, Day } from '../types'
import { svg, n } from '../svg'
import { canvas, label, sized } from '../frame'
import { chartPanel, host, nothing } from '../chart'
import { rampLegend } from '../legend'
import { withTips, type Tips } from '../tip'
import { RAMP_STEPS } from '../palette'
import { stops } from './heatmap'

const DAY = 86_400_000
const LEFT = 34
/** A day's square, the same at every width: a calendar's height followed its
 *  width, and redrawing it at its real width changed its height inside its
 *  own resize — a loop WebKit reports. A panel narrower than a year scrolls. */
const CELL = 13
const YEAR = CELL * 7 + 42
const WIDE = LEFT + 53 * CELL + 4
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const WEEK = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
/** Every other weekday named down the left, Monday first. */
const SIDE = ['Mon', '', 'Wed', '', 'Fri', '', '']

/**
 * A calendar: a measure per day, a year a block of weeks with Monday at the
 * top, each day shaded by the quantile it falls in. What it shows is the shape
 * of a working week and of a season, which a line through the same days
 * smooths away. A day with nothing in it is an outline — "none" and "nearly
 * none" must not look alike — and the shades are keyed underneath.
 */
export function calendarBlock(b: ChartBlock): HTMLElement {
  const panel = chartPanel(b.title)
  const days = b.days ?? []
  const firstDay = days[0]
  const lastDay = days[days.length - 1]
  if (!firstDay || !lastDay) return nothing(panel)

  const tips = withTips(panel)
  const at = host(panel)
  at.classList.add('scrolls')
  const byDate = new Map(days.map((d) => [d.date, d]))
  const first = Number(firstDay.date.slice(0, 4))
  const last = Number(lastDay.date.slice(0, 4))
  let entered = false
  sized(at, (width) => {
    const root = canvas(Math.max(width, WIDE), (last - first + 1) * YEAR + 4)
    for (let y = first; y <= last; y++) year(root, y, (y - first) * YEAR + 34, CELL, byDate, tips)
    if (!entered) root.classList.add('enter')
    entered = true
    at.replaceChildren(root)
  })
  const key = rampLegend(stops(days))
  if (key) panel.append(key)
  return panel
}

/* A year's block is its name on a line of its own — beside the months it ran
   into January — the months' names under it, then seven rows of days. */

/** One year's days from top, a row a weekday and a column a week. */
function year(into: SVGSVGElement, y: number, top: number, s: number, byDate: Map<string, Day>, tips: Tips) {
  into.append(label(0, top - 22, String(y), 'value', 'start'))
  SIDE.forEach((w, i) => { if (w) into.append(label(LEFT - 6, top + i * s + s * 0.72, w, 'tick', 'end')) })
  const jan1 = Date.UTC(y, 0, 1)
  const offset = (new Date(jan1).getUTCDay() + 6) % 7
  for (let t = jan1; new Date(t).getUTCFullYear() === y; t += DAY) {
    const d = new Date(t)
    const col = Math.floor((Math.round((t - jan1) / DAY) + offset) / 7)
    const x = LEFT + col * s
    const day = byDate.get(d.toISOString().slice(0, 10))
    const step = Math.min(day?.step ?? 0, RAMP_STEPS - 1) + 1
    const box = svg('rect', { class: day ? `cell s${step}` : 'cell none', part: 'day',
      x: n(x + 0.5), y: n(top + ((d.getUTCDay() + 6) % 7) * s + 0.5), width: n(s - 2), height: n(s - 2), rx: 2 })
    if (day) {
      box.style.fill = `var(--cr-ramp-${step})`
      tips.bind(box, `${WEEK[d.getUTCDay()]} ${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]} ${y}`, day.formatted)
    }
    into.append(box)
    if (d.getUTCDate() === 1 && x + 24 < into.viewBox.baseVal.width) {
      into.append(label(x, top - 6, MONTHS[d.getUTCMonth()] ?? '', 'tick', 'start'))
    }
  }
}
