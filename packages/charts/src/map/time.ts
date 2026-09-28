import type { FrameMark, Frames, GeoMap, Shape } from '../types'
import { el } from '../dom'
import { svg } from '../svg'

/**
 * A map that plays through time.
 *
 * The map opens on every period together, as paper prints it; a period is a
 * view of it the reader asks for, and "All periods" goes back. A period is
 * drawn as the map is — the same layers, taking a map whose marks carry that
 * period's values — so nothing here knows how a layer draws.
 */

/** How long a whole play takes, about, and the least and most a period
 *  stays on the map within it: a dozen months a second apart, a year of days
 *  a good deal faster. */
const WHOLE_MS = 30_000
const FASTEST_MS = 120
const SLOWEST_MS = 1_100

/**
 * The map in one of its periods: each mark with its value then, against one
 * set of shades and one size for every period. A region with nothing in the
 * period is still drawn, empty — it is still there; a place or a flow with
 * nothing in it is not.
 */
export function frameOf(m: GeoMap, i: number): GeoMap {
  const period = m.frames?.[i] ?? ''
  return {
    ...m,
    shapes: m.shapes.map((s) => shaded(s, s.frames?.[i], period)),
    lines: m.lines?.map((s) => shaded(s, s.frames?.[i], period)),
    markers: present(m.markers, i, (p, v) => ({ ...p, value: v.v, formatted: v.f, weight: v.w ?? 0, size: v.z })),
    arcs: present(m.arcs, i, (a, v) => ({ ...a, value: v.v, formatted: v.f, weight: v.w ?? 0 })),
    legend: m.frameLegend ?? m.legend,
  }
}

function shaded(s: Shape, v: FrameMark | undefined, period: string): Shape {
  return v
    ? { ...s, value: v.v, formatted: v.f, step: v.s ?? 0 }
    : { ...s, value: 0, formatted: `Nothing in ${period}`, step: -1 }
}

function present<T extends { frames?: Frames }>(list: T[], i: number, at: (x: T, v: FrameMark) => T): T[] {
  const out: T[] = []
  for (const x of list) {
    const v = x.frames?.[i]
    if (v) out.push(at(x, v))
  }
  return out
}

/** What a timed map's bubbles are sized against in a period: the values of
 *  every period, since that is what the server weighed them against. */
export function periodValues(m: GeoMap): number[] {
  if (!m.layers.includes('bubble')) return []
  const out: number[] = []
  for (const p of m.markers) {
    for (const v of Object.values(p.frames ?? {})) out.push(v.v)
  }
  return out
}

/** The control under a map that plays through time. */
export interface Timeline {
  element: HTMLElement
  /** Stops playing: the map is gone, or the reader took over. */
  stop(): void
}

/**
 * Play, a slider through the periods, the period shown, and "All periods".
 * `show` is handed a period's place in `periods`, or null for all of them.
 *
 * The slider is a native range: arrow keys step a period, Home and End go to
 * the ends, and a screen reader hears the period's name rather than its
 * index. Dragging it, or choosing all periods, stops the play — the reader
 * has taken over.
 */
export function timeline(periods: string[], show: (i: number | null) => void): Timeline {
  const n = periods.length
  const step = Math.min(SLOWEST_MS, Math.max(FASTEST_MS, WHOLE_MS / n))
  // What all of them together cover, beside the button that shows them.
  const whole = n > 1 ? `${periods[0]} – ${periods[n - 1]}` : periods[0] ?? ''
  const spoken = `All periods, ${whole}`
  const glyph = svg('path', { d: PLAY })
  const play = el('button', { type: 'button', class: 'play', part: 'play', 'aria-label': 'Play through the periods' },
    svg('svg', { viewBox: '0 0 16 16', width: '14', height: '14', 'aria-hidden': 'true' }, glyph))
  const range = el('input', {
    type: 'range', min: '0', max: String(n - 1), step: '1', value: '0', class: 'idle', part: 'period-slider',
    'aria-label': 'Period', 'aria-valuetext': spoken,
  })
  const said = el('output', { class: 'period', part: 'period' }, whole)
  const all = el('button', { type: 'button', class: 'all', part: 'all-periods', 'aria-pressed': 'true' }, 'All periods')

  let at: number | null = null
  let timer: ReturnType<typeof setInterval> | undefined
  const go = (i: number | null) => {
    at = i
    const name = i === null ? whole : periods[i] ?? ''
    range.value = String(i ?? 0)
    range.classList.toggle('idle', i === null)
    range.setAttribute('aria-valuetext', i === null ? spoken : name)
    said.textContent = name
    all.setAttribute('aria-pressed', String(i === null))
    show(i)
  }
  const stop = () => {
    clearInterval(timer)
    timer = undefined
    glyph.setAttribute('d', PLAY)
    play.setAttribute('aria-label', 'Play through the periods')
  }
  const start = () => {
    if (at === null || at >= n - 1) go(0)
    glyph.setAttribute('d', PAUSE)
    play.setAttribute('aria-label', 'Pause')
    timer = setInterval(() => {
      // A map taken out of the page stops, rather than playing to nobody.
      if (!play.isConnected || at === null || at >= n - 1) return stop()
      go(at + 1)
    }, step)
  }
  play.addEventListener('click', () => (timer ? stop() : start()))
  range.addEventListener('input', () => {
    stop()
    go(Number(range.value))
  })
  all.addEventListener('click', () => {
    stop()
    go(null)
  })
  const element = el('div', { class: 'geo-time', part: 'timeline', role: 'group', 'aria-label': 'Periods' },
    play, range, said, all)
  return { element, stop }
}

const PLAY = 'M4.5 2.5v11l9-5.5z'
const PAUSE = 'M4 2.5h3v11H4zm5 0h3v11H9z'
