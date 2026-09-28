import type { GeoMap } from '../types'
import { el } from '../dom'
import { markersOf } from './density'
import { bubbleRadius } from './layers'
import type { View } from './view'

/** One name on the map: where it goes, what it says, and how much it
 *  matters when two want the same room. A place's sits beside its mark; a
 *  region's in its middle. */
export interface Named {
  x: number
  y: number
  text: string
  weight: number
  beside: boolean
  /** How far from its place a name beside one starts, in pixels: clear of
   *  the mark, which for a bubble is its radius. */
  gap?: number
}

/** How many names a view holds at most: past this a map is a page of words. */
const MOST = 150
/** The type they are set in, for measuring: 11px, about this wide a letter. */
const LETTER = 6.4
const LINE = 14

/**
 * Names on the map: regions at their middle, places beside themselves — as
 * many as fit without one covering another, the largest first, so a view of a
 * continent names its biggest few and a street names everything.
 *
 * In HTML over the map rather than SVG text, which would be set in world
 * units: a font a few billionths of the world tall, scaled up. Chosen again
 * each time the map settles, and carried along with it while it moves.
 * Hidden from assistive technology: every mark already says its own name.
 */
export function labeller(layer: HTMLElement, first: Named[]) {
  const box = el('div', { class: 'geo-labels', 'aria-hidden': 'true' })
  layer.append(box)
  let items = [...first].sort((a, b) => b.weight - a.weight)
  let shown: [Named, HTMLElement][] = []

  const place = (n: Named, e: HTMLElement, view: View, scale: number) => {
    e.style.transform = `translate(${Math.round((n.x - view.x) * scale)}px, ${Math.round((n.y - view.y) * scale)}px)`
  }
  const choose = (view: View, scale: number, width: number, height: number) => {
    const taken: [number, number, number, number][] = []
    const next: [Named, HTMLElement][] = []
    for (const n of items) {
      if (next.length >= MOST) break
      const px = (n.x - view.x) * scale
      const py = (n.y - view.y) * scale
      const w = n.text.length * LETTER + 4
      const left = n.beside ? px + (n.gap ?? 7) : px - w / 2
      const top = py - LINE / 2
      if (left < 0 || top < 0 || left + w > width || top + LINE > height) continue
      if (taken.some(([l, t, r, b]) => left < r && left + w > l && top < b && top + LINE > t)) continue
      taken.push([left, top, left + w, top + LINE])
      const e = el('span', { class: n.beside ? 'geo-label beside' : 'geo-label' }, n.text)
      if (n.beside) e.style.marginLeft = `${n.gap ?? 7}px`
      next.push([n, e])
    }
    box.replaceChildren(...next.map(([, e]) => e))
    shown = next
    for (const [n, e] of shown) place(n, e, view, scale)
  }
  return {
    update(view: View, scale: number, settled: boolean, width: number, height: number) {
      if (settled) choose(view, scale, width, height)
      else for (const [n, e] of shown) place(n, e, view, scale)
    },
    swap(next: Named[]) {
      items = [...next].sort((a, b) => b.weight - a.weight)
    },
  }
}

/**
 * What a map names: its regions, and its places — a large map's where a cell
 * is one place, since a crowd has no one name. Only the layers that draw them.
 */
export function namesOf(m: GeoMap): Named[] {
  const out: Named[] = []
  if (m.layers.includes('polygon')) {
    for (const s of m.shapes) {
      const at = anchorOf(s.path)
      if (at && s.label) out.push({ x: at[0], y: at[1], text: s.label, weight: s.value, beside: false })
    }
  }
  if (m.layers.some((l) => l === 'scatter' || l === 'bubble' || l === 'cluster' || l === 'radius')) {
    // A region and the place in it often share a name — a zone and its depot
    // — and a map naming it twice says it once too often.
    const said = new Set(out.map((n) => n.text))
    const bubbles = !m.cells && m.layers.includes('bubble')
    const places = m.cells ? markersOf(m.cells).filter((p) => (p.n ?? 1) === 1) : m.markers
    for (const p of places) {
      if (!p.label || said.has(p.label)) continue
      out.push({ x: p.x, y: p.y, text: p.label, weight: p.value, beside: true,
        gap: bubbles ? bubbleRadius(p.weight) + 3 : 7 })
    }
  }
  return out
}

/**
 * Where a region's name goes: the middle of its largest ring — its centroid,
 * or, for a ring shaped so the centroid falls outside it, the middle of the
 * widest span across it at that height.
 */
export function anchorOf(path: string): [number, number] | null {
  let best: [number, number][] | null = null
  let most = 0
  for (const sub of path.split('Z')) {
    const ring = pointsOf(sub)
    const a = Math.abs(area(ring))
    if (ring.length >= 3 && a > most) {
      best = ring
      most = a
    }
  }
  if (!best) return null
  const c = centroid(best)
  return inside(c, best) ? c : across(c[1], best) ?? best[0] ?? null
}

function pointsOf(sub: string): [number, number][] {
  const out: [number, number][] = []
  for (const pair of sub.split(/[ML]/)) {
    const [x, y] = pair.trim().split(/\s+/).map(Number)
    if (x !== undefined && y !== undefined && Number.isFinite(x) && Number.isFinite(y)) out.push([x, y])
  }
  return out
}

function area(ring: [number, number][]): number {
  let a = 0
  for (let i = 0; i < ring.length; i++) {
    const [x1, y1] = ring[i]!
    const [x2, y2] = ring[(i + 1) % ring.length]!
    a += x1 * y2 - x2 * y1
  }
  return a / 2
}

function centroid(ring: [number, number][]): [number, number] {
  const a = area(ring)
  if (a === 0) return ring[0]!
  let cx = 0
  let cy = 0
  for (let i = 0; i < ring.length; i++) {
    const [x1, y1] = ring[i]!
    const [x2, y2] = ring[(i + 1) % ring.length]!
    const f = x1 * y2 - x2 * y1
    cx += (x1 + x2) * f
    cy += (y1 + y2) * f
  }
  return [cx / (6 * a), cy / (6 * a)]
}

function inside([px, py]: [number, number], ring: [number, number][]): boolean {
  let hit = false
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [xi, yi] = ring[i]!
    const [xj, yj] = ring[j]!
    if ((yi > py) !== (yj > py) && px < ((xj - xi) * (py - yi)) / (yj - yi) + xi) hit = !hit
  }
  return hit
}

/** The middle of the widest stretch of the ring's inside along a line. */
function across(y: number, ring: [number, number][]): [number, number] | null {
  const xs: number[] = []
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [xi, yi] = ring[i]!
    const [xj, yj] = ring[j]!
    if ((yi > y) !== (yj > y)) xs.push(((xj - xi) * (y - yi)) / (yj - yi) + xi)
  }
  xs.sort((a, b) => a - b)
  let best: [number, number] | null = null
  let widest = 0
  for (let k = 0; k + 1 < xs.length; k += 2) {
    const w = xs[k + 1]! - xs[k]!
    if (w > widest) {
      widest = w
      best = [(xs[k]! + xs[k + 1]!) / 2, y]
    }
  }
  return best
}
