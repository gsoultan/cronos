import { el } from '../dom'
import { PLOT_PALETTE_SIZE, RAMP_STEPS, slotOf } from '../palette'
import type { View } from './view'

/** What a painted mark says when pointed at. */
export interface Hit {
  label: string
  sub: string
}

/** The theme's colours, resolved to values a canvas can paint with. */
export interface Colours {
  series(slot: number | undefined): string
  /** A point with no category: the colour .pin is. */
  pin: string
  /** The deepest step of the ramp, which a density field is drawn in. */
  ramp: string
  /** Every step of the ramp, lightest first: a density field's colours. */
  ramps: string[]
  surface: string
}

/**
 * A layer painted on the map's canvas rather than built of elements.
 *
 * Fifty thousand cells as SVG circles is a document a browser spends seconds
 * laying out and a pan it redraws at a few frames a second. Painted, they are
 * pixels — and a pointer finds the mark under it by asking, rather than by an
 * element per mark listening.
 */
export interface Painter {
  paint(ctx: CanvasRenderingContext2D, view: View, scale: number, size: [number, number],
    colours: Colours): void
  /** The mark under a point on the stage, in CSS pixels. */
  find(px: number, py: number): Hit | null
}

/** The canvas over the map and every painter on it. */
export interface Plane {
  element: HTMLCanvasElement
  add(p: Painter): void
  paint(view: View, scale: number, width: number, height: number, settled: boolean): void
  find(px: number, py: number): Hit | null
}

/**
 * The map's canvas.
 *
 * Painted in full once a gesture has stopped. During one, the last full paint
 * is moved and scaled to follow the view: fifty thousand marks painted sixty
 * times a second is a pan that stutters on the phone it most needs to work on,
 * and a picture of them moved is the same picture until the reader lets go.
 */
export function plane(stage: HTMLElement): Plane {
  const element = el('canvas', { class: 'geo-canvas', part: 'map-canvas', 'aria-hidden': 'true' })
  const probe = el('i', { class: 'geo-probe', hidden: '' })
  stage.append(probe)
  const painters: Painter[] = []
  let last: { image: HTMLCanvasElement; view: View; scale: number } | null = null

  const full = (view: View, scale: number, width: number, height: number) => {
    const ctx = fit(element, width, height)
    if (!ctx) return
    const colours = resolve(probe)
    for (const p of painters) p.paint(ctx, view, scale, [width, height], colours)
    last = { image: copy(element), view, scale }
  }

  return {
    element,
    add: (p) => { painters.push(p) },
    paint(view, scale, width, height, settled) {
      if (!(scale > 0) || width <= 0 || height <= 0 || painters.length === 0) return
      if (settled || !last) {
        full(view, scale, width, height)
        return
      }
      const ctx = fit(element, width, height)
      if (!ctx) return
      // Where the last paint's top-left is now, and how much larger it is.
      const k = scale / last.scale
      ctx.drawImage(last.image, (last.view.x - view.x) * scale, (last.view.y - view.y) * scale,
        (last.image.width / dpr()) * k, (last.image.height / dpr()) * k)
    },
    find(px, py) {
      for (let i = painters.length - 1; i >= 0; i--) {
        const hit = painters[i]?.find(px, py)
        if (hit) return hit
      }
      return null
    },
  }
}

const dpr = () => Math.min(globalThis.devicePixelRatio || 1, 2)

/** Sizes the canvas for the stage at the screen's density and clears it. */
function fit(c: HTMLCanvasElement, width: number, height: number): CanvasRenderingContext2D | null {
  const d = dpr()
  const w = Math.round(width * d)
  const h = Math.round(height * d)
  if (c.width !== w || c.height !== h) {
    c.width = w
    c.height = h
  }
  const ctx = c.getContext('2d')
  if (!ctx) return null
  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.clearRect(0, 0, w, h)
  ctx.setTransform(d, 0, 0, d, 0, 0)
  return ctx
}

function copy(c: HTMLCanvasElement): HTMLCanvasElement {
  const out = document.createElement('canvas')
  out.width = c.width
  out.height = c.height
  out.getContext('2d')?.drawImage(c, 0, 0)
  return out
}

/**
 * The theme's colours as values a canvas takes.
 *
 * Read through an element's computed colour rather than the custom property
 * itself: a property can be a var() of another or a color-mix(), and only the
 * computed colour has resolved both. Read at every full paint, so a host that
 * switches to its dark theme is painted in it the next time the map settles.
 */
function resolve(probe: HTMLElement): Colours {
  const read = (v: string) => {
    probe.style.color = v
    return getComputedStyle(probe).color || '#888'
  }
  const series = Array.from({ length: PLOT_PALETTE_SIZE }, (_, i) => read(`var(--cr-series-${i + 1})`))
  return {
    series: (slot) => series[slotOf(slot ?? 0, PLOT_PALETTE_SIZE) - 1] ?? '#888',
    pin: read('var(--cr-series-2)'),
    ramp: read(`var(--cr-ramp-${RAMP_STEPS})`),
    ramps: Array.from({ length: RAMP_STEPS }, (_, i) => read(`var(--cr-ramp-${i + 1})`)),
    surface: read('var(--cr-surface)'),
  }
}
