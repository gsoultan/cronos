import type { Bounds } from '../types'

/** What part of the world is on screen, in world units: the SVG's viewBox. */
export interface View {
  x: number
  y: number
  w: number
  h: number
}

/**
 * The window a map is looked at through.
 *
 * World units throughout. The server projects every coordinate into Web
 * Mercator's unit square, and an XYZ tile grid is defined in the same square,
 * so the SVG's viewBox and the tiles under it are one set of numbers — panning
 * is subtracting from x, and zooming is dividing w. No projection happens in
 * the browser, which is why no map library does either.
 */
export interface Viewport {
  /** The window, as a viewBox. Replaced rather than mutated, so a caller
   *  holding the last one can tell whether it changed. */
  view(): View
  /** CSS pixels per world unit. Zero until the stage has a size. */
  scale(): number
  /** Whether the reader has moved the map since it was last fitted. */
  moved(): boolean
  /** Tells the window how big the stage is. Refits an unmoved map, and keeps
   *  a moved one where the reader left it. */
  size(width: number, height: number): void
  /** Fits the data, as the map first arrived. */
  fit(): void
  /** Frames a box — a cluster's members — as closely as the limits allow. */
  show(b: Bounds): void
  /** Zooms by factor about a point on the stage, in CSS pixels from its
   *  top-left — the centre when none is given. */
  zoom(factor: number, px?: number, py?: number): void
  /** Moves the map by a drag of dx, dy CSS pixels. */
  pan(dx: number, dy: number): void
  /** Whether zooming in (1) or out (-1) would change anything. */
  can(direction: 1 | -1): boolean
}

/**
 * A viewport over bounds, zooming no deeper than tile zoom `deepest` for tiles
 * `tileSize` pixels across.
 *
 * Out as far as the whole world and no further: past it there is nothing to
 * draw, and a map that can be zoomed out to a dot is one a reader can lose.
 */
export function viewport(bounds: Bounds, deepest: number, tileSize = 256): Viewport {
  let width = 0
  let height = 0
  let moved = false
  let view: View = {
    x: bounds.minX, y: bounds.minY,
    w: Math.max(bounds.maxX - bounds.minX, 1e-9), h: Math.max(bounds.maxY - bounds.minY, 1e-9),
  }

  // The widest window whose height is still inside the world, and the
  // narrowest at which a tile is drawn no deeper than the source serves.
  const widest = () => Math.min(1, width / height)
  const narrowest = () => width / (tileSize * 2 ** deepest)

  const clampWidth = (w: number) => Math.min(Math.max(w, narrowest()), widest())

  // Kept inside the world. Anything outside it is a grey band where no tile
  // exists, which reads as the basemap failing to load.
  const place = (x: number, y: number, w: number) => {
    const h = (w * height) / width
    view = {
      x: Math.min(Math.max(x, 0), Math.max(0, 1 - w)),
      y: Math.min(Math.max(y, 0), Math.max(0, 1 - h)),
      w, h,
    }
  }

  // A window of the requested width if the limits allow it, centred where
  // the request was.
  const settle = (next: View) => {
    const w = clampWidth(next.w)
    place(next.x + next.w / 2 - w / 2, next.y + next.h / 2 - ((w * height) / width) / 2, w)
  }

  const frame = (b: Bounds) => {
    if (width <= 0 || height <= 0) return
    const bw = Math.max(b.maxX - b.minX, 1e-9)
    const bh = Math.max(b.maxY - b.minY, 1e-9)
    const s = Math.min(width / bw, height / bh)
    const w = width / s
    const h = height / s
    settle({ x: (b.minX + b.maxX) / 2 - w / 2, y: (b.minY + b.maxY) / 2 - h / 2, w, h })
  }

  const fit = () => {
    frame(bounds)
    moved = false
  }

  return {
    view: () => view,
    scale: () => (width > 0 ? width / view.w : 0),
    moved: () => moved,
    size(w, h) {
      if (w <= 0 || h <= 0 || (w === width && h === height)) return
      const cx = view.x + view.w / 2
      const cy = view.y + view.h / 2
      const s = width > 0 ? width / view.w : 0
      width = w
      height = h
      if (!moved || s === 0) {
        fit()
        return
      }
      // The same place at the same scale, in a box of a different size — a
      // phone turned sideways shows more of the map, not a different one.
      settle({ x: cx - w / s / 2, y: cy - h / s / 2, w: w / s, h: h / s })
    },
    fit,
    show(b) {
      frame(b)
      moved = true
    },
    zoom(factor, px = width / 2, py = height / 2) {
      if (width <= 0 || !(factor > 0)) return
      // The point under the cursor stays under the cursor: work out which
      // world point it is, scale the window, and put that point back.
      const fx = px / width
      const fy = py / height
      const wx = view.x + fx * view.w
      const wy = view.y + fy * view.h
      const w = clampWidth(view.w / factor)
      place(wx - fx * w, wy - fy * ((w * height) / width), w)
      moved = true
    },
    pan(dx, dy) {
      if (width <= 0) return
      const s = width / view.w
      place(view.x - dx / s, view.y - dy / s, view.w)
      moved = true
    },
    can(direction) {
      if (width <= 0) return false
      return direction > 0 ? view.w > narrowest() * 1.0001 : view.w < widest() * 0.9999
    },
  }
}
