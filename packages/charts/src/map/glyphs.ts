import { svg } from '../svg'
import { PLOT_PALETTE_SIZE, slotOf } from '../palette'
import { g } from './geo'

/**
 * The shape a category's places are drawn in, beside its colour: a circle, a
 * square, a triangle — one for each slot of the plot palette.
 *
 * Colour alone leaves a reader who cannot tell two hues apart with a map of
 * identical dots, and one in twelve men is that reader. The shape tells the
 * categories apart without the colour, and costs everybody else nothing. Each
 * is drawn at the circle's area, so no category looks like more.
 */
export type Glyph = 'circle' | 'square' | 'triangle'

const GLYPHS: Glyph[] = ['circle', 'square', 'triangle']

/** Half the side of a square of the unit circle's area. */
const HALF_SIDE = Math.sqrt(Math.PI) / 2
/** The side of an equilateral triangle of the unit circle's area. */
const SIDE = Math.sqrt((4 * Math.PI) / Math.sqrt(3))

/** The glyph for a category's slot, folding past the palette as colour does. */
export function glyphOf(slot: number | undefined): Glyph {
  return GLYPHS[slotOf(slot ?? 0, PLOT_PALETTE_SIZE) - 1] ?? 'circle'
}

/** A glyph placed at x, y in world units, and how to size it: r is the radius
 *  of the circle it stands for, in world units. */
export interface GlyphMark {
  el: SVGElement
  size(r: number): void
}

export function glyphMark(glyph: Glyph, x: number, y: number, attrs: Record<string, string>): GlyphMark {
  if (glyph === 'square') {
    const el = svg('rect', attrs)
    return {
      el,
      size(r) {
        const h = r * HALF_SIDE
        el.setAttribute('x', g(x - h))
        el.setAttribute('y', g(y - h))
        el.setAttribute('width', g(2 * h))
        el.setAttribute('height', g(2 * h))
      },
    }
  }
  if (glyph === 'triangle') {
    const el = svg('polygon', attrs)
    return {
      el,
      size(r) {
        el.setAttribute('points', triangle(x, y, r).map(([px, py]) => `${g(px)},${g(py)}`).join(' '))
      },
    }
  }
  const el = svg('circle', { ...attrs, cx: g(x), cy: g(y), r: '0' })
  return { el, size: (r) => el.setAttribute('r', g(r)) }
}

/** Paints a glyph on a canvas, centred on px, py, standing for a circle of
 *  radius r — all in pixels. */
export function paintGlyph(ctx: CanvasRenderingContext2D, glyph: Glyph, px: number, py: number, r: number) {
  ctx.beginPath()
  if (glyph === 'square') {
    const h = r * HALF_SIDE
    ctx.rect(px - h, py - h, 2 * h, 2 * h)
    return
  }
  if (glyph === 'triangle') {
    const [a, b, c] = triangle(px, py, r)
    ctx.moveTo(a[0], a[1])
    ctx.lineTo(b[0], b[1])
    ctx.lineTo(c[0], c[1])
    ctx.closePath()
    return
  }
  ctx.arc(px, py, r, 0, Math.PI * 2)
}

/** An equilateral triangle, point up, centred on its centroid. */
function triangle(x: number, y: number, r: number): [[number, number], [number, number], [number, number]] {
  const a = r * SIDE
  const h = (a * Math.sqrt(3)) / 2
  return [[x, y - (2 * h) / 3], [x - a / 2, y + h / 3], [x + a / 2, y + h / 3]]
}
