import type { Arc, Marker, Shape } from '../types'
import { svg } from '../svg'
import type { Tips } from '../tip'
import { PLOT_PALETTE_SIZE, RAMP_STEPS, slotOf } from '../palette'
import { g } from './geo'
import type { View } from './view'
import type { Pick } from './sets'
import { glyphMark, glyphOf, type GlyphMark } from './glyphs'

/**
 * One layer of a map, drawn in world units.
 *
 * `update` is for the parts a reader sees at a fixed size in pixels — a dot,
 * a blur, a cluster — which is a different number of world units at every
 * zoom. Without it a bubble grew with every step in until it covered the city
 * it was placed on. Called on every frame of a gesture, and again with
 * `settled` once the gesture stops, for work too heavy to do sixty times a
 * second.
 */
export interface Layer {
  node: SVGElement
  update?(view: View, scale: number, settled: boolean): void
}

/** A step of the sequential ramp, as the custom property the host themes. */
export const ramp = (step: number) =>
  `var(--cr-ramp-${Math.min(Math.max(step, 0), RAMP_STEPS - 1) + 1})`

/** A category's colour. The server caps slots at the plot palette already;
 *  folding again here keeps a newer server's wider cap from painting past it. */
export const series = (slot = 0) => `var(--cr-series-${slotOf(slot, PLOT_PALETTE_SIZE)})`

/** Filled areas from the ramp: regions, or hexagons. */
export function areas(shapes: Shape[], tips: Tips, kind: 'shapes' | 'hexes', pick?: Pick): Layer {
  const node = svg('g', { class: kind })
  for (const s of shapes) {
    const path = svg('path', { d: s.path, part: kind === 'shapes' ? 'region' : 'hexagon', fill: ramp(s.step) })
    tips.bind(path, s.label, pickable(path, s.label, s.formatted, kind === 'shapes' ? pick : undefined))
    node.append(path)
  }
  return { node }
}

/**
 * Makes a mark set the report's filter to its label, when the map sets one:
 * a click or Enter picks it, and picks it again to let go. Returns what the
 * tooltip says under the value — what a click would do.
 */
export function pickable(mark: Element, label: string, value: string, pick?: Pick): string {
  if (!pick) return value
  const on = pick.picked(label)
  mark.classList.add('pickable')
  if (on) mark.classList.add('picked')
  // A button, pressed while it is the filter: what a screen reader needs to
  // say that a click does something, and what it did.
  mark.setAttribute('role', 'button')
  mark.setAttribute('aria-pressed', String(on))
  mark.addEventListener('click', () => pick.toggle(label))
  mark.addEventListener('keydown', (e) => {
    const k = (e as KeyboardEvent).key
    if (k !== 'Enter' && k !== ' ') return
    e.preventDefault()
    pick.toggle(label)
  })
  return `${value} · ${on ? 'click to show everything again' : 'click to filter the report to it'}`
}

/**
 * Routes, coloured from the ramp.
 *
 * Each over a casing in the surface colour, one stroke wider: a light step of
 * the ramp on a light street map is otherwise a line nobody can find, and the
 * casing is also what the pointer catches — a three-pixel line is a hard
 * thing to hover.
 */
export function routes(lines: Shape[], tips: Tips): Layer {
  const node = svg('g', { class: 'routes' })
  for (const l of lines) {
    const route = svg('path', { d: l.path, class: 'route' })
    // A style and not a stroke attribute: an attribute is the weakest thing
    // in the cascade, and any rule that styles a path — ours drew every route
    // in the surface colour — wins over it without a word.
    route.style.stroke = ramp(l.step)
    const one = svg('g', { part: 'route' }, svg('path', { d: l.path, class: 'casing' }), route)
    tips.bind(one, l.label, l.formatted)
    node.append(one)
  }
  return { node }
}

let gradients = 0

/**
 * The density layer: every point a soft disc, the discs adding up where the
 * points crowd together.
 *
 * A radial gradient rather than a blur filter. One gradient serves every disc
 * and is painted with it, where a filter renders the whole layer again into a
 * buffer of its own on every frame of a pan — on the main thread of our
 * customer's customer's page.
 *
 * The radius is pixels, so a hot spot is the same size on screen at every zoom
 * and the field sharpens into its points as the reader zooms in.
 */
export function heat(markers: Marker[]): Layer {
  const id = `cr-heat-${++gradients}`
  const colour = `var(--cr-ramp-${RAMP_STEPS})`
  const circles = markers.map((p) => svg('circle', {
    cx: g(p.x), cy: g(p.y), r: '0', fill: `url(#${id})`,
    // Opacity and not radius carries the weight. A radius that shrank with
    // the value would say a quiet place is a small place, and the two are
    // different claims.
    opacity: String(Math.round((0.18 + p.weight * 0.6) * 1000) / 1000),
  }))
  const node = svg('g', {},
    svg('radialGradient', { id },
      svg('stop', { offset: '0', 'stop-color': colour, 'stop-opacity': '0.85' }),
      svg('stop', { offset: '0.5', 'stop-color': colour, 'stop-opacity': '0.35' }),
      svg('stop', { offset: '1', 'stop-color': colour, 'stop-opacity': '0' })),
    svg('g', { class: 'heat' }, ...circles))

  let last = 0
  return {
    node,
    update(_view, scale) {
      if (scale === last || !(scale > 0)) return
      last = scale
      const r = g(26 / scale)
      for (const c of circles) c.setAttribute('r', r)
    },
  }
}

/**
 * A dot per point, or a bubble per point sized by its value.
 *
 * Area, not radius, carries the size — see scatter.ts. The floor keeps a
 * near-zero value visible, because a marker that renders as nothing is
 * indistinguishable from a row that was filtered away. Bubbles are drawn
 * largest first, so a small one is never hidden under a large neighbour.
 */
export function dots(markers: Marker[], tips: Tips, sized: boolean, keyed: boolean, pick?: Pick): Layer {
  const node = svg('g', { class: 'dots' })
  const order = sized ? [...markers].sort((a, b) => b.weight - a.weight) : markers
  const drawn: Sized[] = []
  for (const p of order) {
    drawn.push([pin(node, p, tips, keyed, pick), sized ? bubbleRadius(p.weight) : 4.5])
  }
  return { node, update: radii(drawn) }
}

/** A bubble's radius in pixels for a weight: area, not radius, carries the
 *  value, over a floor that keeps a near-zero one visible. */
export const bubbleRadius = (weight: number) => 4 + Math.sqrt(Math.max(weight, 0)) * 14

/** A place's mark: a circle, or its category's glyph when the map colours by
 *  one — see glyphs.ts. Returns how to size it. */
export function pin(node: SVGElement, p: Marker, tips: Tips, keyed: boolean, pick?: Pick): GlyphMark['size'] {
  const mark = glyphMark(keyed ? glyphOf(p.slot) : 'circle', p.x, p.y, { class: 'pin', part: 'marker' })
  if (keyed) mark.el.style.fill = series(p.slot)
  tips.bind(mark.el, p.label, pickable(mark.el, p.label, p.size ? `${p.formatted} · ${p.size}` : p.formatted, pick))
  node.append(mark.el)
  return mark.size
}

/** A mark and its size in pixels. */
export type Sized = [GlyphMark['size'], number]

/** Keeps marks their size in pixels as the scale changes. */
export function radii(drawn: Sized[]): NonNullable<Layer['update']> {
  let last = 0
  return (_view, scale) => {
    if (scale === last || !(scale > 0)) return
    last = scale
    for (const [size, px] of drawn) size(px / scale)
  }
}

/**
 * Flows, as arcs rather than straight lines.
 *
 * Two depots that trade in both directions produce two segments on exactly the
 * same line, and one hides the other. Bowing each one to the left of its own
 * direction of travel separates them, and makes the direction readable without
 * an arrowhead at every scale. The width is pixels, set once: a non-scaling
 * stroke keeps it so at every zoom.
 */
export function flows(arcs: Arc[], tips: Tips, keyed: boolean): Layer & { swap(arcs: Arc[]): void } {
  const node = svg('g', { class: 'flows' })
  const lay = (list: Arc[]) => {
    node.replaceChildren()
    for (const a of list) node.append(arc(a, tips, keyed))
  }
  lay(arcs)
  // The routes of a view a large map's reader moved to.
  return { node, swap: lay }
}

function arc(a: Arc, tips: Tips, keyed: boolean): SVGElement {
  const dx = a.x2 - a.x1
  const dy = a.y2 - a.y1
  const bow = 0.18
  const cx = (a.x1 + a.x2) / 2 - dy * bow
  const cy = (a.y1 + a.y2) / 2 + dx * bow
  const path = svg('path', {
    d: `M${g(a.x1)} ${g(a.y1)}Q${g(cx)} ${g(cy)} ${g(a.x2)} ${g(a.y2)}`,
    class: 'flow', part: 'flow',
  })
  path.style.strokeWidth = `${Math.round((1.5 + a.weight * 4.5) * 10) / 10}px`
  path.style.stroke = keyed ? series(a.slot) : 'var(--cr-series-1)'
  tips.bind(path, a.label, a.formatted)
  return path
}
