import type { Marker } from '../types'
import { el } from '../dom'
import { svg } from '../svg'
import type { Tips } from '../tip'
import { cluster, count, placesIn, type Cluster } from './cluster'
import { g } from './geo'
import { radii, series, type Layer } from './layers'

/** How close two points may be on screen, in CSS px, before they are one.
 *  Wider than the widest circle, so two clusters never overlap. */
const CELL = 72

/**
 * The cluster layer: points that would overlap drawn as one counted circle,
 * separating as the reader zooms in.
 *
 * The counts are buttons in an HTML layer over the map rather than SVG. A
 * button is focusable and answers Enter without a line of code, and its label
 * is text at the size the page sets — SVG text in world units is a font a few
 * millionths of a unit tall, which is the size browsers stop drawing
 * correctly. The points on their own stay in the SVG with every other dot.
 *
 * Regrouping waits for a gesture to stop. During one the circles move with
 * the map and keep their groups, so a pinch does not reshuffle forty buttons
 * sixty times a second.
 */
export function clusters(first: Marker[], tips: Tips, keyed: boolean, overlay: HTMLElement,
  show: (c: Cluster) => boolean): Layer & { swap(markers: Marker[]): void } {

  let markers = first

  const node = svg('g', { class: 'dots' })
  let bubbles: [HTMLButtonElement, Cluster][] = []
  let size: NonNullable<Layer['update']> = () => {}
  let groupedAt = 0

  const regroup = (scale: number) => {
    groupedAt = scale
    const drawn: [SVGCircleElement, number][] = []
    node.replaceChildren()
    for (const [b] of bubbles) b.remove()
    bubbles = []

    for (const c of cluster(markers, CELL / scale)) {
      const one = c.members[0]
      if (placesIn(c) === 1 && one) {
        const mark = svg('circle', { cx: g(one.x), cy: g(one.y), r: '0', class: 'pin', part: 'marker' })
        if (keyed) mark.style.fill = series(one.slot)
        tips.bind(mark, one.label, one.formatted)
        node.append(mark)
        drawn.push([mark, 4.5])
        continue
      }
      bubbles.push([bubble(c, tips, show), c])
    }
    overlay.append(...bubbles.map(([b]) => b))
    size = radii(drawn)
  }

  return {
    node,
    // The cells of a view a reader moved to: grouped again at the next
    // update, whatever the scale.
    swap(next) {
      markers = next
      groupedAt = 0
    },
    update(view, scale, settled) {
      if (!(scale > 0)) return
      if (groupedAt === 0 || (settled && Math.abs(Math.log2(scale / groupedAt)) > 0.01)) regroup(scale)
      size(view, scale, settled)
      for (const [b, c] of bubbles) {
        b.style.transform = `translate(${(c.x - view.x) * scale}px,${(c.y - view.y) * scale}px) translate(-50%,-50%)`
      }
    },
  }
}

/** One cluster's button. */
function bubble(c: Cluster, tips: Tips, show: (c: Cluster) => boolean): HTMLButtonElement {
  const n = placesIn(c)
  const d = Math.round(Math.min(24 + Math.log2(n) * 5, 60))
  const b = el('button', {
    type: 'button', class: 'cluster', part: 'cluster',
    'aria-label': `${n} locations. Zoom in to separate them.`,
    style: `width:${d}px;height:${d}px`,
  }, count(n))

  const say = (e: { clientX: number; clientY: number }) =>
    tips.show(e as PointerEvent, `${n} locations`, 'Click to zoom in')
  b.addEventListener('pointerenter', (e) => say(e))
  b.addEventListener('pointerleave', () => tips.hide())
  b.addEventListener('focus', () => {
    const r = b.getBoundingClientRect()
    say({ clientX: r.left + r.width / 2, clientY: r.top })
  })
  b.addEventListener('blur', () => tips.hide())
  b.addEventListener('click', (e) => {
    tips.hide()
    // At the deepest zoom, points at one address never separate — five
    // parcels to the same warehouse are one place on any map. Saying who is
    // in there is the only way left to answer the click.
    if (!show(c)) {
      const names = c.members.slice(0, 4).map((m) => m.label).join(', ')
      const more = n - Math.min(c.members.length, 4)
      tips.show(e as PointerEvent, `${n} locations here`, more > 0 ? `${names} and ${more} more` : names)
    }
  })
  return b
}
