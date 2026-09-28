import type { ChartBlock, DrawOptions, GeoMap } from '../types'
import { el } from '../dom'
import { svg } from '../svg'
import { rampLegend } from '../legend'
import { withTips, type Tips } from '../tip'
import { PLOT_PALETTE_SIZE, slotOf } from '../palette'
import { viewport, type Viewport } from '../map/view'
import { tileLayer } from '../map/tiles'
import { areas, dots, flows, heat, routes, type Layer } from '../map/layers'
import { clusters } from '../map/clusters'
import { controls } from '../map/controls'
import { credits } from '../map/credits'
import { around, viewBox } from '../map/geo'
import { plane, type Plane } from '../map/plane'
import { density, markersOf, type Density } from '../map/density'
import { refiner, type Refiner } from '../map/detail'
import { grouped } from '../map/format'
import { bubbleValues, diverge, keyLegend, SHADED, sizeLegend } from '../map/keys'
import { areaTools, pickOf, type Pick } from '../map/sets'

/** How deep a map with no basemap may be zoomed: a street, about. */
const DEEPEST = 18

/** The layers a large map paints on its canvas rather than builds. */
const PAINTED = new Set(['heat', 'bubble', 'scatter'])

/** A layer, and how it takes the places of a view a large map's reader moved
 *  to — for the layers that draw places at all. */
type Drawn = Layer & { take?(m: GeoMap): void }

/**
 * A map.
 *
 * No map library, and no tile-loading library either. The server projects
 * every geometry to Web Mercator normalised to the unit square — which is the
 * space an XYZ tile grid is already defined in — so the whole map is an SVG
 * whose viewBox is the part of the world in view, over an `<img>` per tile at
 * a position worked out with two multiplications. Panning moves the viewBox
 * and zooming shrinks it; nothing is re-projected.
 *
 * Layers are drawn in the order the server listed them, so an author who asks
 * for polygons under dots gets polygons under dots. A large map's places are
 * the exception: they are painted on a canvas over the shapes, because fifty
 * thousand of them are pixels rather than elements, and asked for again from
 * the server as the reader zooms — see map/detail.ts.
 */
export function mapBlock(b: ChartBlock, opts: DrawOptions = {}): HTMLElement {
  const panel = el('section', { class: 'panel wide', part: 'panel' }, el('h3', {}, b.title))
  const m = b.map
  if (!m || m.layers.length === 0) {
    panel.append(el('p', { class: 'unaffected' }, 'Nothing to place on a map.'))
    return panel
  }
  if (empty(m)) {
    panel.append(el('p', { class: 'unaffected' }, 'No data in this period.'))
    return panel
  }

  const tips = withTips(panel)
  const tiles = m.tiles
  const port = viewport(m.bounds, tiles?.maxZoom ?? DEEPEST, tiles?.tileSize ?? 256)
  const canvas = svg('svg', { class: 'geo', part: 'chart', preserveAspectRatio: 'none' })
  const overlay = el('div', { class: 'geo-overlay' })
  const stage = el('div', {
    class: tiles ? 'geo-stage tiled' : 'geo-stage', tabindex: '0', role: 'group',
    'aria-label': `${b.title}. Drag or use the arrow keys to move the map, plus and minus to zoom.`,
    // The data's own proportions, within reason. A country the shape of
    // Chile would otherwise be a panel taller than the screen.
    style: `--geo-aspect:${aspectOf(m)}`,
  })

  const basemap = tiles ? tileLayer(tiles) : null
  if (basemap) stage.append(basemap.element)
  stage.append(canvas)
  // The map and whatever it draws over itself, bottom to top.
  const maps = [m, ...(m.overlays ?? [])]
  // Only where a layer paints: clusters of cells are buttons, not pixels.
  const painted = maps.some((x) => x.cells && x.layers.some((l) => PAINTED.has(l))) ? plane(stage) : null
  if (painted) stage.append(painted.element)
  // Over the painted places, so a heat field does not bury what is drawn on it.
  const above = maps.length > 1 || m.area
    ? svg('svg', { class: 'geo-above', part: 'chart-above', preserveAspectRatio: 'none' })
    : null
  if (above) stage.append(above)
  stage.append(overlay)
  if (tiles?.logo) {
    stage.append(el('img', { class: 'geo-logo', part: 'logo', src: tiles.logo, alt: tiles.logoAlt ?? '' }))
  }

  // Frames a cluster's members, and says whether that got any closer.
  const show = (c: { members: { x: number; y: number }[] }) => {
    const before = port.view().w
    port.show(around(c.members))
    redraw(true)
    return port.view().w < before * 0.99
  }
  // Only the map itself sets the report's filters; what it draws over itself
  // is there to be read.
  const pick = pickOf(m, opts.filter)
  const layers: Drawn[] = []
  const refiners: Refiner[] = []
  // What the map's own bubbles are sized by, keyed again as a view arrives.
  const sizes = el('div', { class: 'size-slot' })
  for (const [k, x] of maps.entries()) {
    const tint = k > 0 && !x.keys?.length ? tintOf(k) : undefined
    const own: Drawn[] = []
    for (const name of x.layers) {
      const l = draw(name, x, { tips, overlay, show, painted, pick: k === 0 ? pick : undefined, tint })
      if (!l) continue
      if (x.ramp === 'diverging' && SHADED.has(name)) diverge(l.node)
      if (tint !== undefined) l.node.style.setProperty('--cr-pin', `var(--cr-series-${slotOf(tint, PLOT_PALETTE_SIZE)})`)
      ;(k > 0 && above ? above : canvas).append(l.node)
      own.push(l)
    }
    layers.push(...own)
    if (x.detail && opts.mapView) {
      refiners.push(refiner(x.detail, opts.mapView, stage, (got) => {
        for (const l of own) l.take?.(got)
        if (k === 0) sizes.replaceChildren(sizeLegend(bubbleValues({ ...x, ...got })) ?? '')
        redraw(true)
      }))
    }
  }
  sizes.replaceChildren(sizeLegend(bubbleValues(m)) ?? '')
  areaTools(stage, above ?? canvas, port, m.area, opts.filter)

  const line = credits(tiles, m.note)
  let frame = 0
  let settledNext = false
  const render = () => {
    frame = 0
    const settled = settledNext
    settledNext = false
    const view = port.view()
    const scale = port.scale()
    canvas.setAttribute('viewBox', viewBox(view))
    above?.setAttribute('viewBox', viewBox(view))
    for (const l of layers) l.update?.(view, scale, settled)
    painted?.paint(view, scale, stage.clientWidth, stage.clientHeight, settled)
    if (basemap) basemap.lay(view, stage.clientWidth, stage.clientHeight)
    if (settled) {
      line.follow(view, basemap?.zoom() ?? -1)
      for (const r of refiners) r.follow(view)
    }
    buttons.refresh()
  }
  // One draw a frame however many events asked for one, and a settled draw
  // wins over an unsettled one asked for in the same frame. `now` draws before
  // the next paint, for a size change: a frame drawn at the old proportions
  // is a map stretched sideways for a sixtieth of a second, which is visible.
  const redraw = (settled: boolean, now = false) => {
    settledNext ||= settled
    if (now) {
      cancelAnimationFrame(frame)
      render()
      return
    }
    if (!frame) frame = requestAnimationFrame(render)
  }
  const buttons = controls(stage, port, redraw, () => tips.hide())
  if (painted) pointAt(stage, painted, tips, pick)

  panel.append(stage)
  legends(panel, m, line.element, sizes)
  watch(stage, port, redraw, () => { for (const r of refiners) r.stop() })
  canvas.setAttribute('viewBox', viewBox(port.view()))
  above?.setAttribute('viewBox', viewBox(port.view()))
  return panel
}

/** What goes under a map: the ramp, the categories, the credits, and what the
 *  reader should know about how much of the data is on it. */
function legends(panel: HTMLElement, m: GeoMap, credit: HTMLElement | null, sizes: HTMLElement) {
  const ramp = ramped(m) ? rampLegend(m.legend) : null
  if (ramp && m.ramp === 'diverging') diverge(ramp)
  if (ramp) panel.append(ramp)
  const keyed = keyLegend(m)
  if (keyed) panel.append(keyed)
  panel.append(sizes)
  if (credit) panel.append(credit)
  // Beside the legend, where a reader checks what the colours mean: a map
  // drawn from part of its data has totals that mean less than they look.
  if (m.partial) panel.append(el('p', { class: 'unaffected', part: 'partial' }, m.partial))
  if (m.places) {
    // Zooming in reaches the places themselves only on a map that can be
    // asked for more — a hexagon is the same hexagon at every zoom.
    panel.append(el('p', { class: 'unaffected', part: 'places' }, m.detail
      ? `${grouped(m.places)} places, gathered where they crowd together. Zoom in to see each one.`
      : `${grouped(m.places)} places, every one of them counted.`))
  }
  for (const [k, ov] of (m.overlays ?? []).entries()) overlayLegend(panel, ov, k + 1)
}

/**
 * What a dataset drawn over the map says under it. Places in one colour are
 * that colour beside its name — without it, a reader cannot tell its dots
 * from the map's own. Places coloured by category, or shapes shaded, are its
 * name over what the colours mean.
 */
function overlayLegend(panel: HTMLElement, ov: GeoMap, k: number) {
  const ramp = ramped(ov) ? rampLegend(ov.legend) : null
  if (ramp && ov.ramp === 'diverging') diverge(ramp)
  const keys = keyLegend(ov)
  if (!keys && pinned(ov)) {
    panel.append(el('div', { class: 'legend', part: 'legend overlay-key' },
      el('span', { class: 'key' },
        el('i', { class: 'swatch dot', style: `background: var(--cr-series-${slotOf(tintOf(k), PLOT_PALETTE_SIZE)})` }),
        ov.title ?? '')))
  }
  if (ramp || keys) {
    panel.append(el('p', { class: 'legend-title', part: 'overlay-title' }, ov.title ?? ''))
    if (ramp) panel.append(ramp)
    if (keys) panel.append(keys)
  }
  if (ov.partial) panel.append(el('p', { class: 'unaffected', part: 'partial' }, ov.partial))
}

/** Whether a map draws places as pins — the marks a colour tells apart. */
function pinned(m: GeoMap): boolean {
  const places = m.markers.length + (m.cells?.x.length ?? 0) > 0
  return places && m.layers.some((l) => l === 'scatter' || l === 'bubble' || l === 'cluster')
}

/**
 * The palette slot the kth dataset drawn over a map paints its places in:
 * never the map's own pin colour, and from the slots that stay apart for
 * every reader. Past them it folds into the last, as a series does — see
 * palette.ts — rather than a hue two readers in twelve cannot tell apart.
 */
function tintOf(k: number): number {
  const slots = [2, 0]
  return slots[Math.min(k - 1, slots.length - 1)] ?? 0
}

/** What the layers need besides the map. */
interface Scene {
  tips: Tips
  overlay: HTMLElement
  show: (c: { members: { x: number; y: number }[] }) => boolean
  painted: Plane | null
  pick?: Pick
  /** The palette slot an overlay's places are painted in. */
  tint?: number
}

/** One layer, or nothing for a layer this build has never heard of. */
function draw(layer: string, m: GeoMap, s: Scene): Drawn | undefined {
  const keyed = (m.keys?.length ?? 0) > 0
  if (m.cells && s.painted && PAINTED.has(layer)) {
    const kind = layer as 'heat' | 'bubble' | 'scatter'
    return paintedLayer(density(m.cells, kind, { keyed, tint: s.tint, picks: !!s.pick }), s.painted)
  }
  switch (layer) {
    case 'polygon':
      return areas(m.shapes, s.tips, 'shapes', s.pick)
    case 'hexbin':
      return areas(m.hexes ?? [], s.tips, 'hexes')
    case 'line':
      return routes(m.lines ?? [], s.tips)
    case 'heat':
      return heat(m.markers)
    case 'cluster': {
      const c = clusters(m.cells ? markersOf(m.cells) : m.markers, s.tips, keyed, s.overlay, s.show, s.pick)
      return { ...c, take: (got) => { if (got.cells) c.swap(markersOf(got.cells)) } }
    }
    case 'bubble':
      return dots(m.markers, s.tips, true, keyed, s.pick)
    case 'scatter':
      return dots(m.markers, s.tips, false, keyed, s.pick)
    case 'flow': {
      const f = flows(m.arcs, s.tips, keyed)
      return { ...f, take: (got) => f.swap(got.arcs) }
    }
  }
  // A layer this build has never heard of is a normal condition — the server
  // and the viewer ship separately. Drawing the layers it does know beats
  // refusing the whole map.
  return undefined
}

/** A painted layer, as a layer: its node is an empty group so the SVG keeps
 *  the author's order for everything else, and its places are on the plane. */
function paintedLayer(d: Density, on: Plane): Drawn {
  on.add(d)
  return { node: svg('g', {}), take: (got) => { if (got.cells) d.replace(got.cells) } }
}

/**
 * The painted marks answer a pointer by being asked where it is: there is no
 * element under it to listen. Over a mark, its tooltip; off one, whatever the
 * elements beneath had to say.
 */
function pointAt(stage: HTMLElement, on: Plane, tips: Tips, pick?: Pick) {
  let showing = false
  // A painted place, clicked, picks it — the drag guard in controls.ts stops
  // this from running at the end of a drag.
  // A mark drawn over the map answers for itself, above the painted places.
  const own = (e: Event) => e.target instanceof HTMLButtonElement ||
    (e.target instanceof Element && e.target.closest('.geo-above > g') !== null)
  if (pick) {
    stage.addEventListener('click', (e) => {
      if (own(e)) return
      const box = stage.getBoundingClientRect()
      const hit = on.find(e.clientX - box.left, e.clientY - box.top)
      if (hit?.place) pick.toggle(hit.place)
    })
  }
  stage.addEventListener('pointermove', (e) => {
    if (stage.classList.contains('dragging') || own(e)) return
    const box = stage.getBoundingClientRect()
    const hit = on.find(e.clientX - box.left, e.clientY - box.top)
    if (hit) {
      tips.show(e, hit.label, hit.sub)
      showing = true
    } else if (showing) {
      tips.hide()
      showing = false
    }
  })
}

/**
 * Sizes the map to its stage, and keeps it sized.
 *
 * Observed rather than measured once: the panel has no size until it is in
 * the document, and a host page that opens a drawer or a phone turned
 * sideways changes it afterwards. A stage taken out of the document reports a
 * size of nothing, which is when the observer lets go of it — and when a
 * large map stops asking for views nobody will see.
 */
function watch(stage: HTMLElement, port: Viewport, redraw: (settled: boolean, now?: boolean) => void,
  gone: () => void) {
  if (typeof ResizeObserver === 'undefined') return
  const seen = new ResizeObserver((entries) => {
    const box = entries[0]?.contentRect
    if (!stage.isConnected) {
      seen.disconnect()
      gone()
      return
    }
    if (!box || box.width <= 0 || box.height <= 0) return
    port.size(box.width, box.height)
    redraw(true, true)
  })
  seen.observe(stage)
}

/** Whether there is anything at all to draw, on the map or over it. */
function empty(m: GeoMap): boolean {
  const own = m.shapes.length + m.markers.length + m.arcs.length +
    (m.lines?.length ?? 0) + (m.hexes?.length ?? 0) + (m.cells?.x.length ?? 0)
  return own === 0 && (m.overlays ?? []).every(empty)
}

/** Whether a layer shaded from the ramp drew anything, which is when the
 *  legend explaining the ramp belongs under the map. */
function ramped(m: GeoMap): boolean {
  return (m.layers.includes('polygon') && m.shapes.length > 0) ||
    (m.layers.includes('line') && (m.lines?.length ?? 0) > 0) ||
    (m.layers.includes('hexbin') && (m.hexes?.length ?? 0) > 0)
}

/** The stage's proportions: the data's, between a square and a wide strip. */
function aspectOf(m: GeoMap): string {
  const w = m.bounds.maxX - m.bounds.minX
  const h = m.bounds.maxY - m.bounds.minY
  const a = w > 0 && h > 0 ? w / h : 1.6
  return String(Math.round(Math.min(Math.max(a, 1), 2.2) * 1000) / 1000)
}

