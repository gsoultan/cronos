import { svg, n } from './svg'

/**
 * A chart drawn at its real size.
 *
 * Charts were drawn once, in a coordinate space a hundred units wide, and
 * stretched to their panel by CSS. A circle became an ellipse at every width
 * but one, a stroke smeared sideways into a grey bar, and nothing knew how
 * wide a label would be — so the last one ran off the edge, and a scatter
 * plot shrank into a strip down the middle of its panel. Now the SVG is
 * exactly as many pixels as it shows, and it is drawn again when the panel's
 * width changes: a few hundred elements at most, which costs less than the
 * reflow that announced it.
 *
 * The first draw happens at once, at a guess, so a chart has its marks the
 * moment it exists — before it is in a page, and in a browser that has not
 * laid it out yet. The real width follows as soon as there is one.
 */
export function sized(host: HTMLElement, draw: (width: number) => void, guess = 640): () => void {
  let drawn = 0
  const at = (width: number) => {
    const w = Math.floor(width)
    if (w < 40 || w === drawn) return
    drawn = w
    draw(w)
  }
  at(guess)
  if (typeof ResizeObserver !== 'undefined') {
    const seen = new ResizeObserver((entries) => {
      // A panel taken out of the page lets go of its observer, rather than
      // keeping it for as long as the page lives.
      if (!host.isConnected) return seen.disconnect()
      at(entries[0]?.contentRect.width ?? 0)
    })
    seen.observe(host)
  }
  // Again at the width it has: for a change that is not a resize.
  return () => draw(drawn)
}

/** Measures text as it will be drawn: in the host's font, at a pixel size. */
export type Measure = (text: string, px: number) => number

let pen: CanvasRenderingContext2D | null | undefined

/**
 * A measure in the font the host has, once it is in a page. Before then, and
 * where a canvas cannot be had, an average advance that errs wide: a label
 * given too much room is a gap, and one given too little is a clipped number.
 */
export function measure(host: Element): Measure {
  pen ??= typeof document === 'undefined' ? null : document.createElement('canvas').getContext('2d')
  const family = host.isConnected ? getComputedStyle(host).fontFamily : ''
  const ctx = pen
  if (!ctx || !family) return (text, px) => text.length * px * 0.62
  return (text, px) => {
    ctx.font = `${px}px ${family}`
    return ctx.measureText(text).width
  }
}

/**
 * Text cut to a width, with an ellipsis where it was cut. The whole of it
 * goes in the mark's tooltip; a label is a pointer to it, not the record.
 */
export function fit(text: string, width: number, px: number, m: Measure): string {
  if (m(text, px) <= width) return text
  let lo = 0
  let hi = text.length
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2)
    if (m(`${text.slice(0, mid)}…`, px) <= width) lo = mid
    else hi = mid - 1
  }
  return lo > 0 ? `${text.slice(0, lo).trimEnd()}…` : ''
}

/** An SVG of exactly width × height pixels. */
export function canvas(width: number, height: number, cls = 'canvas'): SVGSVGElement {
  return svg('svg', {
    width, height, viewBox: `0 0 ${width} ${height}`, class: cls, part: 'chart',
    // The marks carry their own labels and the heading names the whole; a
    // screen reader gets numbers from the marks rather than "graphic".
    'aria-hidden': 'true',
  })
}

/** A label in the SVG, in one of the chart sheet's text roles. */
export function label(x: number, y: number, text: string, cls: string,
  anchor: 'start' | 'middle' | 'end' = 'middle'): SVGTextElement {
  return svg('text', { x: n(x), y: n(y), class: cls, 'text-anchor': anchor }, text)
}

/**
 * A bar's outline, rounded at the end its value is at and square where it
 * stands on the axis: a bar rounded at both ends floats, and one rounded at
 * neither reads as a block of colour rather than a length.
 *
 * `end` is the side the value is on: up for a column above zero, down for one
 * below it, right and left for a bar.
 */
export function bar(x: number, y: number, w: number, h: number,
  end: 'up' | 'down' | 'right' | 'left', radius = 4): string {
  if (w <= 0 || h <= 0) return ''
  const r = Math.min(radius, (end === 'up' || end === 'down' ? w : h) / 2,
    end === 'up' || end === 'down' ? h : w)
  const x2 = x + w
  const y2 = y + h
  switch (end) {
    case 'up':
      return `M${n(x)} ${n(y2)}V${n(y + r)}Q${n(x)} ${n(y)} ${n(x + r)} ${n(y)}H${n(x2 - r)}Q${n(x2)} ${n(y)} ${n(x2)} ${n(y + r)}V${n(y2)}Z`
    case 'down':
      return `M${n(x)} ${n(y)}V${n(y2 - r)}Q${n(x)} ${n(y2)} ${n(x + r)} ${n(y2)}H${n(x2 - r)}Q${n(x2)} ${n(y2)} ${n(x2)} ${n(y2 - r)}V${n(y)}Z`
    case 'right':
      return `M${n(x)} ${n(y)}H${n(x2 - r)}Q${n(x2)} ${n(y)} ${n(x2)} ${n(y + r)}V${n(y2 - r)}Q${n(x2)} ${n(y2)} ${n(x2 - r)} ${n(y2)}H${n(x)}Z`
    default:
      return `M${n(x2)} ${n(y)}H${n(x + r)}Q${n(x)} ${n(y)} ${n(x)} ${n(y + r)}V${n(y2 - r)}Q${n(x)} ${n(y2)} ${n(x + r)} ${n(y2)}H${n(x2)}Z`
  }
}

/**
 * A band of a ring from angle a0 to a1, in radians clockwise from twelve
 * o'clock, between radii r0 (0 for a pie slice) and r1. Paths rather than
 * dashes on a stroke: a dash cannot be hovered as the slice it looks like, and
 * its gap was a length along the ring, so a thin slice lost more of itself to
 * the gap than a thick one.
 */
export function arc(cx: number, cy: number, r0: number, r1: number, a0: number, a1: number): string {
  const sweep = a1 - a0
  // A whole ring is two halves: an arc's two ends at one point draw nothing.
  if (sweep >= Math.PI * 2 - 1e-6) {
    const mid = a0 + Math.PI
    return arc(cx, cy, r0, r1, a0, mid) + arc(cx, cy, r0, r1, mid, a1)
  }
  const large = sweep > Math.PI ? 1 : 0
  const px = (r: number, a: number) => `${n(cx + r * Math.sin(a))} ${n(cy - r * Math.cos(a))}`
  const outer = `M${px(r1, a0)}A${n(r1)} ${n(r1)} 0 ${large} 1 ${px(r1, a1)}`
  if (r0 <= 0) return `${outer}L${n(cx)} ${n(cy)}Z`
  return `${outer}L${px(r0, a1)}A${n(r0)} ${n(r0)} 0 ${large} 0 ${px(r0, a0)}Z`
}

/**
 * The size a figure is set at to fit across a width: its largest while it
 * fits, smaller as it grows. Estimated from its length rather than measured,
 * because these are the figures drawn in a fixed box — a donut's hole, a
 * dial's middle — and digits run about six tenths of their size.
 */
export function sizeToFit(text: string, width: number, largest: number): number {
  return Math.max(10, Math.min(largest, width / Math.max(text.length * 0.6, 1)))
}

/** A unique id within the page, for the gradients and clips a chart defines. */
let ids = 0
export const uid = (prefix: string) => `cr-${prefix}-${++ids}`
