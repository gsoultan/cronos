import type { Arc, Marker, Shape } from '../types'
import { svg } from '../svg'
import { say, type Tips } from '../tip'
import { PLOT_PALETTE_SIZE, RAMP_STEPS, slotOf } from '../palette'
import { g, latitude, longitude } from './geo'
import type { View } from './view'
import { geodesic, pathOf, type Pick } from './sets'
import { glyphMark, glyphOf, type GlyphMark } from './glyphs'
import { grouped } from './format'

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

/**
 * Filled areas from the ramp: regions, or hexagons.
 *
 * `shade` shades the same areas with other values — a period of a map that
 * plays through time — in place: a region's outline is the heavy part, and
 * it is the same in every period. One with a step below zero had nothing in
 * the period, and is drawn empty rather than left off the map it is still on.
 */
export function areas(shapes: Shape[], tips: Tips, kind: 'shapes' | 'hexes', pick?: Pick): Layer & { shade(shapes: Shape[]): void } {
  const node = svg('g', { class: kind })
  let now = shapes
  const drawn = shapes.map((s, i) => {
    const path = svg('path', { d: s.path, part: kind === 'shapes' ? 'region' : 'hexagon' })
    const says = pickable(path, s.label, kind === 'shapes' ? pick : undefined)
    tips.bind(path, s.label, () => says(now[i]?.formatted ?? ''))
    node.append(path)
    return { path, says }
  })
  const shade = (list: Shape[]) => {
    now = list
    for (const [i, { path, says }] of drawn.entries()) {
      const s = list[i]
      if (!s) continue
      path.setAttribute('fill', ramp(s.step))
      path.classList.toggle('void', s.step < 0)
      say(path, s.label, says(s.formatted))
    }
  }
  shade(shapes)
  return { node, shade }
}

/**
 * Makes a mark set the report's filter to its label, when the map sets one:
 * a click or Enter picks it, and picks it again to let go. Returns what the
 * tooltip says under a value — what a click would do.
 */
export function pickable(mark: Element, label: string, pick?: Pick): (value: string) => string {
  if (!pick) return (value) => value
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
  const does = on ? 'click to show everything again' : 'click to filter the report to it'
  return (value) => `${value} · ${does}`
}

/**
 * Routes, coloured from the ramp.
 *
 * Each over a casing in the surface colour, one stroke wider: a light step of
 * the ramp on a light street map is otherwise a line nobody can find, and the
 * casing is also what the pointer catches — a three-pixel line is a hard
 * thing to hover.
 */
export function routes(lines: Shape[], tips: Tips): Layer & { shade(lines: Shape[]): void } {
  const node = svg('g', { class: 'routes' })
  let now = lines
  const drawn = lines.map((l, i) => {
    const route = svg('path', { d: l.path, class: 'route' })
    const one = svg('g', { part: 'route' }, svg('path', { d: l.path, class: 'casing' }), route)
    tips.bind(one, l.label, () => now[i]?.formatted ?? '')
    node.append(one)
    return { one, route }
  })
  // Recoloured in place for a period, as areas are.
  const shade = (list: Shape[]) => {
    now = list
    for (const [i, { one, route }] of drawn.entries()) {
      const l = list[i]
      if (!l) continue
      // A style and not a stroke attribute: an attribute is the weakest
      // thing in the cascade, and any rule that styles a path — ours drew
      // every route in the surface colour — wins over it without a word.
      route.style.stroke = l.step < 0 ? 'var(--cr-line)' : ramp(l.step)
      say(one, l.label, l.formatted)
    }
  }
  shade(lines)
  return { node, shade }
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
export function heat(markers: Marker[]): Layer & { swap(markers: Marker[]): void } {
  const id = `cr-heat-${++gradients}`
  const colour = `var(--cr-ramp-${RAMP_STEPS})`
  const field = svg('g', { class: 'heat' })
  const node = svg('g', {},
    svg('radialGradient', { id },
      svg('stop', { offset: '0', 'stop-color': colour, 'stop-opacity': '0.85' }),
      svg('stop', { offset: '0.5', 'stop-color': colour, 'stop-opacity': '0.35' }),
      svg('stop', { offset: '1', 'stop-color': colour, 'stop-opacity': '0' })),
    field)

  let circles: SVGElement[] = []
  let last = 0
  // The places of a period of a map that plays through time.
  const swap = (list: Marker[]) => {
    circles = list.map((p) => svg('circle', {
      cx: g(p.x), cy: g(p.y), r: '0', fill: `url(#${id})`,
      // Opacity and not radius carries the weight. A radius that shrank with
      // the value would say a quiet place is a small place, and the two are
      // different claims.
      opacity: String(Math.round((0.18 + p.weight * 0.6) * 1000) / 1000),
    }))
    field.replaceChildren(...circles)
    last = 0
  }
  swap(markers)
  return {
    node,
    swap,
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
export function dots(markers: Marker[], tips: Tips, sized: boolean, keyed: boolean, pick?: Pick): Layer & { swap(markers: Marker[]): void } {
  const node = svg('g', { class: 'dots' })
  let size = radii([])
  // The places of a period of a map that plays through time.
  const swap = (list: Marker[]) => {
    node.replaceChildren()
    const order = sized ? [...list].sort((a, b) => b.weight - a.weight) : list
    size = radii(order.map((p) => [pin(node, p, tips, keyed, pick), sized ? bubbleRadius(p.weight) : 4.5]))
  }
  swap(markers)
  return { node, swap, update: (view, scale, settled) => size(view, scale, settled) }
}

/** A bubble's radius in pixels for a weight: area, not radius, carries the
 *  value, over a floor that keeps a near-zero one visible. */
export const bubbleRadius = (weight: number) => 4 + Math.sqrt(Math.max(weight, 0)) * 14

/** A place's mark: a circle, or its category's glyph when the map colours by
 *  one — see glyphs.ts. Returns how to size it. */
export function pin(node: SVGElement, p: Marker, tips: Tips, keyed: boolean, pick?: Pick): GlyphMark['size'] {
  const mark = glyphMark(keyed ? glyphOf(p.slot) : 'circle', p.x, p.y, { class: 'pin', part: 'marker' })
  if (keyed) mark.el.style.fill = series(p.slot)
  tips.bind(mark.el, p.label, pickable(mark.el, p.label, pick)(p.size ? `${p.formatted} · ${p.size}` : p.formatted))
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
 * direction of travel separates them. A head where each lands says which way
 * it goes, sized in pixels like the stroke — which is a non-scaling one, so
 * both stay the same at every zoom. Moving, when the author asks, the dashes
 * run from where a flow starts to where it lands; see styles.ts for the
 * reader who has asked their system for less motion.
 */
export function flows(arcs: Arc[], tips: Tips, keyed: boolean, moving = false): Layer & { swap(arcs: Arc[]): void } {
  const node = svg('g', { class: moving ? 'flows moving' : 'flows' })
  let heads: ((scale: number) => void)[] = []
  let last = 0
  const lay = (list: Arc[]) => {
    node.replaceChildren()
    heads = list.map((a) => arc(a, tips, keyed, node))
    last = 0
  }
  lay(arcs)
  // The routes of a view a large map's reader moved to.
  return {
    node,
    swap: lay,
    update(_view, scale) {
      if (scale === last || !(scale > 0)) return
      last = scale
      for (const h of heads) h(scale)
    },
  }
}

/** One flow and its head, drawn into node. Returns how to size the head. */
function arc(a: Arc, tips: Tips, keyed: boolean, node: SVGElement): (scale: number) => void {
  const dx = a.x2 - a.x1
  const dy = a.y2 - a.y1
  const bow = 0.18
  const cx = (a.x1 + a.x2) / 2 - dy * bow
  const cy = (a.y1 + a.y2) / 2 + dx * bow
  const path = svg('path', {
    d: `M${g(a.x1)} ${g(a.y1)}Q${g(cx)} ${g(cy)} ${g(a.x2)} ${g(a.y2)}`,
    class: 'flow', part: 'flow',
  })
  const width = Math.round((1.5 + a.weight * 4.5) * 10) / 10
  const colour = keyed ? series(a.slot) : 'var(--cr-series-1)'
  path.style.strokeWidth = `${width}px`
  path.style.stroke = colour
  tips.bind(path, a.label, a.formatted)
  const head = svg('polygon', { class: 'flow-head', part: 'flow-head' })
  head.style.fill = colour
  node.append(path, head)

  // The way the arc is heading as it lands: from its control point to its end.
  const l = Math.hypot(a.x2 - cx, a.y2 - cy) || 1
  const ux = (a.x2 - cx) / l
  const uy = (a.y2 - cy) / l
  return (scale) => {
    // Past the end by half the stroke, so the line's round cap is under the
    // head rather than poking out of its point.
    const tip = (width / 2) / scale
    const length = (5 + width * 1.2) / scale
    const half = length / 2
    const tx = a.x2 + ux * tip
    const ty = a.y2 + uy * tip
    const bx = tx - ux * length
    const by = ty - uy * length
    head.setAttribute('points',
      `${g(tx)},${g(ty)} ${g(bx - uy * half)},${g(by + ux * half)} ${g(bx + uy * half)},${g(by - ux * half)}`)
  }
}

/**
 * A circle of the map's radius around each place — a delivery area, a
 * catchment — measured on the ground, so one far north is taller on the map
 * than one at the equator, as the ground it covers is. Translucent, so where
 * two overlap reads as overlap.
 */
export function rings(places: Marker[], km: number, tips: Tips, keyed: boolean): Layer & { swap(places: Marker[]): void } {
  const node = svg('g', { class: 'rings' })
  const lay = (list: Marker[]) => {
    node.replaceChildren()
    // Fewer sides for a crowd: still round at the size each is drawn, and a
    // few thousand circles of sixty-four points is a document, not a map.
    const sides = list.length > 500 ? 24 : 64
    for (const p of list) {
      const ring = svg('path', { d: pathOf(geodesic(latitude(p.y), longitude(p.x), km, sides)), class: 'ring', part: 'radius' })
      if (keyed) ring.style.setProperty('--cr-pin', series(p.slot))
      tips.bind(ring, p.label, `within ${grouped(km)} km`)
      node.append(ring)
    }
  }
  lay(places)
  return { node, swap: lay }
}
