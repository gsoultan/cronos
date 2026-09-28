import type { Cells, Marker } from '../types'
import { compact, grouped } from './format'
import type { Colours, Hit, Painter } from './plane'
import type { View } from './view'

/** The layers a large map paints from its cells. */
export type DensityKind = 'scatter' | 'bubble' | 'heat'

/** How far a place's heat reaches, in CSS pixels: the SVG heat layer's. */
const HEAT = 26

/** The cells a painter draws, replaceable when the reader moves the map. */
export interface Density extends Painter {
  replace(cells: Cells): void
}

/**
 * A layer of a large map, painted from its cells: a dot per cell for scatter,
 * sized by its measure for bubble, a density field for heat.
 *
 * A cell of one place is drawn and described as that place; a cell of several
 * says how many, and is a little larger than a place, so a crowd reads as one
 * before a reader has zoomed far enough to separate it.
 */
export function density(first: Cells, kind: DensityKind, keyed: boolean): Density {
  let cells = first
  let weights = weigh(first)
  let index = new ScreenIndex()
  let palette: { colours: string; rgb: Uint8ClampedArray } | null = null

  return {
    replace(next) {
      cells = next
      weights = weigh(next)
    },
    paint(ctx, view, scale, size, colours) {
      index = new ScreenIndex()
      if (kind === 'heat') {
        const key = colours.ramps.join()
        if (palette?.colours !== key) palette = { colours: key, rgb: gradient(colours.ramps) }
        heat(ctx, { cells, weights }, view, scale, size, palette.rgb)
        return
      }
      dots(ctx, { cells, weights, kind, keyed, index }, view, scale, size, colours)
    },
    find(px, py) {
      if (kind === 'heat') return null
      const i = index.nearest(px, py)
      return i < 0 ? null : describe(cells, i)
    },
  }
}

/** A large map's cells as markers, for the cluster layer: each a place, or a
 *  count of places the clusters add up. */
export function markersOf(c: Cells): Marker[] {
  return c.x.map((x, i) => {
    const hit = describe(c, i)
    return {
      label: hit.label, formatted: hit.sub, x, y: c.y[i] ?? 0, value: c.v[i] ?? 0,
      weight: 0, slot: c.s?.[i], n: c.n[i] ?? 1,
    }
  })
}

/** What a cell says: the place it is, or how many it holds. */
export function describe(c: Cells, i: number): Hit {
  const n = c.n[i] ?? 1
  const v = compact(c.v[i] ?? 0)
  if (n === 1) return { label: c.l?.[i] || '1 location', sub: v }
  return { label: `${grouped(n)} locations`, sub: c.mean ? `${v} on average` : v }
}

/** Each cell's measure as a share of the largest, for a bubble's size and a
 *  heat's strength. Counts where the measure is zero everywhere. */
function weigh(c: Cells): number[] {
  // Loops rather than Math.max(...list): fifty thousand arguments is past
  // what some engines will pass to a function.
  let most = 0
  for (const v of c.v) most = Math.max(most, v)
  let crowd = 1
  for (const n of c.n) crowd = Math.max(crowd, n)
  const of = most > 0 ? c.v : c.n
  const top = most > 0 ? most : crowd
  return of.map((v) => Math.max(0, v) / top)
}

type DotLayer = { cells: Cells; weights: number[]; kind: DensityKind; keyed: boolean; index: ScreenIndex }

function dots(ctx: CanvasRenderingContext2D, d: DotLayer, view: View, scale: number,
  [width, height]: [number, number], colours: Colours) {
  const { cells, weights } = d
  const order = cells.x.map((_, i) => i)
  // Largest first, so a small bubble is never hidden under a large neighbour.
  if (d.kind === 'bubble') order.sort((a, b) => (weights[b] ?? 0) - (weights[a] ?? 0))
  ctx.lineWidth = 1.2
  ctx.strokeStyle = colours.surface
  for (const i of order) {
    const px = ((cells.x[i] ?? 0) - view.x) * scale
    const py = ((cells.y[i] ?? 0) - view.y) * scale
    const n = cells.n[i] ?? 1
    const r = d.kind === 'bubble'
      ? 4 + Math.sqrt(weights[i] ?? 0) * 14
      : n > 1 ? Math.min(4.5 + Math.log2(n) * 0.8, 9) : 4.5
    if (px < -r || py < -r || px > width + r || py > height + r) continue
    ctx.fillStyle = d.keyed ? colours.series(cells.s?.[i]) : colours.pin
    ctx.beginPath()
    ctx.arc(px, py, r, 0, Math.PI * 2)
    ctx.fill()
    ctx.stroke()
    d.index.add(px, py, r, i)
  }
}

/** A density field's resolution: one sample every this many CSS pixels,
 *  smoothed back up to the screen. */
const FIELD = 4

/**
 * The density layer of a large map: every cell's weight spread over the
 * pixels around it, added up, and coloured through the ramp by its share of
 * the densest point in view.
 *
 * Added up in numbers and coloured once, rather than painted splat over
 * splat. Painted, fifty overlapping splats at even a fifth of full strength
 * are one opaque blob — every depot in Europe the same solid disc, which says
 * where the data is and nothing about how much. Scaled to the densest point in
 * view, the field keeps its shape at every zoom: zoomed in on one city, the
 * city's own busiest street is the dark end of the ramp.
 */
function heat(ctx: CanvasRenderingContext2D, d: { cells: Cells; weights: number[] }, view: View,
  scale: number, [width, height]: [number, number], palette: Uint8ClampedArray) {
  const gw = Math.ceil(width / FIELD) + 1
  const gh = Math.ceil(height / FIELD) + 1
  const field = new Float32Array(gw * gh)
  const reach = HEAT / FIELD
  const kernel = gaussian(reach)
  const side = Math.ceil(reach)
  for (let i = 0; i < d.cells.x.length; i++) {
    const gx = (((d.cells.x[i] ?? 0) - view.x) * scale) / FIELD
    const gy = (((d.cells.y[i] ?? 0) - view.y) * scale) / FIELD
    if (gx < -side || gy < -side || gx > gw + side || gy > gh + side) continue
    spread(field, gw, gh, Math.round(gx), Math.round(gy), d.weights[i] ?? 0, kernel, side)
  }
  let most = 0
  for (const v of field) most = Math.max(most, v)
  if (!(most > 0)) return
  const image = new ImageData(gw, gh)
  for (let p = 0; p < field.length; p++) {
    // The square root of the share, so a town is visible beside the city
    // that sets the scale — linear, everything but the densest place is a
    // faint wash.
    const t = Math.sqrt((field[p] ?? 0) / most)
    // The faintest few percent left out, so the field fades into the map
    // rather than tinting all of it.
    if (t < 0.03) continue
    const c = Math.min(255, Math.round(t * 255)) * 4
    image.data[p * 4] = palette[c] ?? 0
    image.data[p * 4 + 1] = palette[c + 1] ?? 0
    image.data[p * 4 + 2] = palette[c + 2] ?? 0
    image.data[p * 4 + 3] = Math.round(255 * Math.min(0.85, 0.25 + t * 0.75))
  }
  const small = document.createElement('canvas')
  small.width = gw
  small.height = gh
  small.getContext('2d')?.putImageData(image, 0, 0)
  ctx.imageSmoothingEnabled = true
  ctx.drawImage(small, 0, 0, gw * FIELD, gh * FIELD)
}

/** Adds one cell's weight to the field around (cx, cy). */
function spread(field: Float32Array, gw: number, gh: number, cx: number, cy: number, w: number,
  kernel: Float32Array, side: number) {
  const span = side * 2 + 1
  for (let dy = -side; dy <= side; dy++) {
    const y = cy + dy
    if (y < 0 || y >= gh) continue
    for (let dx = -side; dx <= side; dx++) {
      const x = cx + dx
      if (x < 0 || x >= gw) continue
      field[y * gw + x] = (field[y * gw + x] ?? 0) + w * (kernel[(dy + side) * span + dx + side] ?? 0)
    }
  }
}

let kernels: Map<number, Float32Array> | undefined

/** A gaussian falling to a few percent at `reach`, one per reach. */
function gaussian(reach: number): Float32Array {
  kernels ??= new Map()
  const have = kernels.get(reach)
  if (have) return have
  const side = Math.ceil(reach)
  const span = side * 2 + 1
  const k = new Float32Array(span * span)
  const sigma = reach / 2.5
  for (let y = -side; y <= side; y++) {
    for (let x = -side; x <= side; x++) {
      k[(y + side) * span + x + side] = Math.exp(-(x * x + y * y) / (2 * sigma * sigma))
    }
  }
  kernels.set(reach, k)
  return k
}

/**
 * The ramp as 256 colours, lightest first: the theme's steps drawn as one
 * gradient and read back, so a field's colours are the ones every other ramp
 * on the page uses — in whatever notation the browser computed them.
 */
function gradient(steps: string[]): Uint8ClampedArray {
  const c = document.createElement('canvas')
  c.width = 256
  c.height = 1
  const ctx = c.getContext('2d')
  if (!ctx) return new Uint8ClampedArray(256 * 4)
  const g = ctx.createLinearGradient(0, 0, 256, 0)
  steps.forEach((s, i) => g.addColorStop(steps.length > 1 ? i / (steps.length - 1) : 0, s))
  ctx.fillStyle = g
  ctx.fillRect(0, 0, 256, 1)
  return ctx.getImageData(0, 0, 256, 1).data
}

/**
 * Where each mark was painted, bucketed by screen position, so the one under
 * the pointer is found by looking in a few buckets rather than at every mark.
 */
export class ScreenIndex {
  #buckets = new Map<number, [number, number, number, number][]>()
  static readonly SIZE = 24

  add(x: number, y: number, r: number, i: number) {
    const key = ScreenIndex.key(Math.floor(x / ScreenIndex.SIZE), Math.floor(y / ScreenIndex.SIZE))
    const list = this.#buckets.get(key)
    if (list) list.push([x, y, r, i])
    else this.#buckets.set(key, [[x, y, r, i]])
  }

  /** The topmost mark whose circle holds the point, or -1. */
  nearest(x: number, y: number): number {
    const bx = Math.floor(x / ScreenIndex.SIZE)
    const by = Math.floor(y / ScreenIndex.SIZE)
    let best = -1
    let bestD = Infinity
    for (let dx = -1; dx <= 1; dx++) {
      for (let dy = -1; dy <= 1; dy++) {
        for (const [mx, my, r, i] of this.#buckets.get(ScreenIndex.key(bx + dx, by + dy)) ?? []) {
          const d = (mx - x) ** 2 + (my - y) ** 2
          if (d <= (r + 2) ** 2 && d <= bestD) {
            best = i
            bestD = d
          }
        }
      }
    }
    return best
  }

  static key(bx: number, by: number): number {
    return (bx + 32768) * 65536 + (by + 32768)
  }
}
