import type { Tiles } from '../types'
import type { View } from './view'
import { el } from '../dom'

/** The basemap under a map, as a layer that follows the view. */
export interface TileLayer {
  element: HTMLElement
  /** Lays out the tiles for a view on a stage of the given size, in CSS px. */
  lay(view: View, width: number, height: number): void
  /** The zoom the tiles are drawn at, for a provider whose credit line
   *  depends on it. */
  zoom(): number
}

/** One zoom level's tiles, and how many of them have yet to arrive. */
interface Level {
  z: number
  box: HTMLElement
  tiles: Map<string, HTMLImageElement>
  waiting: number
}

/**
 * Tiles as `<img>`, laid out by arithmetic.
 *
 * No tile-loading library. A tile (z, x, y) covers the world-unit square
 * [x/2^z, (x+1)/2^z) — the square the server projected the data into — so
 * where a tile goes is two multiplications. A choropleth that pulled in
 * Leaflet would cost more than this entire bundle's budget before drawing
 * anything.
 *
 * A level stays until the next one has arrived. Zooming across a tile
 * boundary asks for a whole new set, and removing the old one first left the
 * reader looking at an empty box for as long as the new one took; kept
 * underneath, it is a blurred map for a moment rather than no map.
 */
export function tileLayer(t: Tiles): TileLayer {
  const element = el('div', { class: 'tiles', part: 'basemap' })
  const size = t.tileSize && t.tileSize > 0 ? t.tileSize : 256
  // The dense tiles on a dense screen. The zoom is chosen in CSS pixels
  // either way — a 2x tile covers the same ground at twice the pixels, so it
  // is sharper rather than further in.
  const template = (globalThis.devicePixelRatio ?? 1) >= 1.5 && t.url2x ? t.url2x : t.url
  const levels = new Map<number, Level>()
  let current = -1

  const prune = () => {
    const top = levels.get(current)
    if (!top || top.waiting > 0) return
    for (const [z, level] of levels) {
      if (z === current) continue
      level.box.remove()
      levels.delete(z)
    }
  }

  const add = (level: Level, key: string, tx: number, ty: number) => {
    const img = el('img', {
      src: template.replaceAll('{z}', String(level.z)).replaceAll('{x}', String(tx))
        .replaceAll('{y}', String(ty)),
      alt: '', loading: 'lazy', decoding: 'async', draggable: 'false',
      // The origin and not the path. The path is our customer's application
      // and often names their customer, which is no tile server's business;
      // the origin is how every one of them tells who is asking. OpenStreetMap
      // blocks a request without it, and a Mapbox token restricted to our
      // customer's domain refuses one — so `no-referrer`, which this was,
      // drew a basemap of "Referer is required" tiles.
      referrerpolicy: 'strict-origin',
    })
    level.waiting++
    const settled = () => {
      if (img.dataset.done) return
      img.dataset.done = '1'
      level.waiting--
      prune()
    }
    img.addEventListener('load', settled)
    img.addEventListener('error', () => {
      // A tile that failed is a hole in the basemap, not a broken-image icon
      // in the middle of our customer's page.
      img.style.visibility = 'hidden'
      settled()
    })
    level.tiles.set(key, img)
    level.box.append(img)
    return img
  }

  const place = (level: Level, view: View, scale: number, create: boolean) => {
    const n = 2 ** level.z
    const clamp = (v: number) => Math.min(Math.max(v, 0), n - 1)
    const x0 = clamp(Math.floor(view.x * n))
    const x1 = clamp(Math.floor((view.x + view.w) * n))
    const y0 = clamp(Math.floor(view.y * n))
    const y1 = clamp(Math.floor((view.y + view.h) * n))

    const keep = new Set<string>()
    for (let tx = x0; tx <= x1; tx++) {
      for (let ty = y0; ty <= y1; ty++) {
        const key = `${tx}/${ty}`
        const img = level.tiles.get(key) ?? (create ? add(level, key, tx, ty) : undefined)
        if (!img) continue
        keep.add(key)
        // Whole pixels, each edge rounded once: two tiles that share an edge
        // share the pixel it lands on, so no seam shows between them.
        const left = Math.round((tx / n - view.x) * scale)
        const top = Math.round((ty / n - view.y) * scale)
        const right = Math.round(((tx + 1) / n - view.x) * scale)
        const bottom = Math.round(((ty + 1) / n - view.y) * scale)
        img.style.cssText = `left:${left}px;top:${top}px;width:${right - left}px;height:${bottom - top}px` +
          (img.style.visibility === 'hidden' ? ';visibility:hidden' : '')
      }
    }
    for (const [key, img] of level.tiles) {
      if (keep.has(key)) continue
      // Settled here, so its load cannot settle it a second time. A removed
      // image goes on loading, and each late arrival took one more off the
      // count — which reached zero while tiles of the new level were still
      // on their way, and took the old level out from under the gap.
      if (!img.dataset.done) {
        img.dataset.done = '1'
        level.waiting--
      }
      img.remove()
      level.tiles.delete(key)
    }
  }

  return {
    element,
    zoom: () => current,
    lay(view, width) {
      const scale = width / view.w
      if (!(scale > 0)) return
      // The zoom whose tiles land nearest their native size: between 0.7 and
      // 1.4 of it, so the basemap is neither blurry nor fetched at four times
      // the detail anybody can see.
      const z = Math.max(0, Math.min(t.maxZoom, Math.round(Math.log2(scale / size))))
      let level = levels.get(z)
      if (!level) {
        level = { z, box: el('div', { class: 'tile-level' }), tiles: new Map(), waiting: 0 }
        levels.set(z, level)
      }
      if (z !== current) {
        current = z
        element.append(level.box) // on top of the level it replaces
      }
      place(level, view, scale, true)
      for (const [lz, other] of levels) {
        if (lz !== z) place(other, view, scale, false)
      }
      prune()
    },
  }
}
