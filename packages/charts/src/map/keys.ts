import type { GeoMap } from '../types'
import { el } from '../dom'
import { svg } from '../svg'
import { PLOT_PALETTE_SIZE, RAMP_STEPS, slotOf } from '../palette'
import { glyphOf } from './glyphs'
import { bubbleRadius } from './layers'
import { compact } from './format'
import { g } from './geo'

/** The layers whose places a category's glyph is drawn on. */
const PLACED = new Set(['scatter', 'bubble', 'cluster'])

/** The layers shaded from the ramp. */
export const SHADED = new Set(['polygon', 'hexbin', 'line', 'h3'])

/**
 * The key for places coloured by category: each its colour and, on a map that
 * draws places, the glyph its places are drawn in — so the key reads without
 * the colour, as the map does. See glyphs.ts.
 */
export function keyLegend(m: GeoMap): HTMLElement | null {
  const keys = m.keys
  if (!keys || keys.length === 0) return null
  const placed = m.layers.some((l) => PLACED.has(l))
  return el('div', { class: 'legend', part: 'legend' },
    ...keys.map((k) =>
      el('span', { class: 'key' },
        el('i', {
          class: `swatch ${placed ? glyphOf(k.slot) : 'circle'}`,
          style: `background: var(--cr-series-${slotOf(k.slot, PLOT_PALETTE_SIZE)})`,
        }),
        k.label)))
}

/**
 * Has an element read a diverging map's two-hue ramp through the one-hue
 * ramp's names — its shaded layers and its legend alike, so every shade is
 * spelled once and a heat field on the same map keeps the ramp it had.
 */
export function diverge(node: HTMLElement | SVGElement) {
  for (let i = 1; i <= RAMP_STEPS; i++) node.style.setProperty(`--cr-ramp-${i}`, `var(--cr-div-${i})`)
}

/**
 * What a bubble's size says: three circles drawn to the rule the bubbles are,
 * the largest at a round number near the biggest value on the map, so a
 * reader holds a bubble against them rather than guessing what its area
 * means. Nothing where a value is below zero, whose bubbles are sized from
 * the lowest value rather than from nothing, or where every value is zero.
 */
export function sizeLegend(values: number[]): HTMLElement | null {
  const scale = sizeSteps(values)
  if (!scale) return null
  return el('div', { class: 'legend sizes', part: 'legend size-key' },
    ...scale.refs.map((v) => {
      const r = bubbleRadius(v / scale.hi)
      const ring = svg('svg', { class: 'size-ring', width: g(2 * r + 2), height: g(2 * r + 2), 'aria-hidden': 'true' },
        svg('circle', { cx: g(r + 1), cy: g(r + 1), r: g(r) }))
      return el('span', { class: 'key' }, ring, compact(v))
    }))
}

/** The values a size legend shows, and the largest value the bubbles are
 *  sized against — or nothing, where a legend would say something false. */
export function sizeSteps(values: number[]): { refs: number[]; hi: number } | null {
  let lo = Infinity
  let hi = 0
  for (const v of values) {
    lo = Math.min(lo, v)
    hi = Math.max(hi, v)
  }
  if (!(hi > 0) || lo < 0) return null
  const top = roundBelow(hi)
  const refs = [...new Set([top, roundBelow(top / 4), roundBelow(top / 20)])].filter((v) => v > 0)
  return { refs, hi }
}

/** The values a map's bubbles are sized by: its places', or its cells'. */
export function bubbleValues(m: GeoMap): number[] {
  if (!m.layers.includes('bubble')) return []
  return m.cells ? m.cells.v : m.markers.map((p) => p.value)
}

/** The largest one, two or five times a power of ten at or below v. */
function roundBelow(v: number): number {
  if (!(v > 0)) return 0
  const mag = 10 ** Math.floor(Math.log10(v))
  const n = v / mag
  return (n >= 5 ? 5 : n >= 2 ? 2 : 1) * mag
}
