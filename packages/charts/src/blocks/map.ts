import type { ChartBlock, GeoMap, MapKey } from '../types'
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

/** How deep a map with no basemap may be zoomed: a street, about. */
const DEEPEST = 18

/**
 * A map.
 *
 * No map library, and no tile-loading library either. The server projects
 * every geometry to Web Mercator normalised to the unit square — which is the
 * space an XYZ tile grid is already defined in — so the whole map is an SVG
 * whose viewBox is the part of the world in view, over an `<img>` per tile at
 * a position worked out with two multiplications. Panning moves the viewBox
 * and zooming shrinks it; nothing is re-projected and nothing is refetched
 * from the server.
 *
 * Layers are drawn in the order the server listed them, so an author who asks
 * for polygons under dots gets polygons under dots.
 */
export function mapBlock(b: ChartBlock): HTMLElement {
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
  stage.append(canvas, overlay)
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
  const layers = m.layers.flatMap((l) => draw(l, m, tips, overlay, show) ?? [])
  for (const l of layers) canvas.append(l.node)

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
    for (const l of layers) l.update?.(view, scale, settled)
    if (basemap) basemap.lay(view, stage.clientWidth, stage.clientHeight)
    if (settled) line.follow(view, basemap?.zoom() ?? -1)
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

  panel.append(stage)
  const ramp = ramped(m) ? rampLegend(m.legend) : null
  if (ramp) panel.append(ramp)
  const keyed = keyLegend(m.keys)
  if (keyed) panel.append(keyed)
  if (line.element) panel.append(line.element)
  // Beside the legend, where a reader checks what the colours mean: a map
  // drawn from part of its data has totals that mean less than they look.
  if (m.partial) panel.append(el('p', { class: 'unaffected', part: 'partial' }, m.partial))

  watch(stage, port, redraw)
  canvas.setAttribute('viewBox', viewBox(port.view()))
  return panel
}

/** One layer, or nothing for a layer this build has never heard of. */
function draw(layer: string, m: GeoMap, tips: Tips, overlay: HTMLElement,
  show: (c: { members: { x: number; y: number }[] }) => boolean): Layer | undefined {
  const keyed = (m.keys?.length ?? 0) > 0
  switch (layer) {
    case 'polygon':
      return areas(m.shapes, tips, 'shapes')
    case 'hexbin':
      return areas(m.hexes ?? [], tips, 'hexes')
    case 'line':
      return routes(m.lines ?? [], tips)
    case 'heat':
      return heat(m.markers)
    case 'cluster':
      return clusters(m.markers, tips, keyed, overlay, show)
    case 'bubble':
      return dots(m.markers, tips, true, keyed)
    case 'scatter':
      return dots(m.markers, tips, false, keyed)
    case 'flow':
      return flows(m.arcs, tips, keyed)
  }
  // A layer this build has never heard of is a normal condition — the server
  // and the viewer ship separately. Drawing the layers it does know beats
  // refusing the whole map.
  return undefined
}

/**
 * Sizes the map to its stage, and keeps it sized.
 *
 * Observed rather than measured once: the panel has no size until it is in
 * the document, and a host page that opens a drawer or a phone turned
 * sideways changes it afterwards. A stage taken out of the document reports a
 * size of nothing, which is when the observer lets go of it.
 */
function watch(stage: HTMLElement, port: Viewport, redraw: (settled: boolean, now?: boolean) => void) {
  if (typeof ResizeObserver === 'undefined') return
  const seen = new ResizeObserver((entries) => {
    const box = entries[0]?.contentRect
    if (!stage.isConnected) {
      seen.disconnect()
      return
    }
    if (!box || box.width <= 0 || box.height <= 0) return
    port.size(box.width, box.height)
    redraw(true, true)
  })
  seen.observe(stage)
}

/** Whether there is anything at all to draw. */
function empty(m: GeoMap): boolean {
  return m.shapes.length + m.markers.length + m.arcs.length +
    (m.lines?.length ?? 0) + (m.hexes?.length ?? 0) === 0
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

/** The key for points coloured by category. */
function keyLegend(keys: MapKey[] | undefined): HTMLElement | null {
  if (!keys || keys.length === 0) return null
  return el('div', { class: 'legend', part: 'legend' },
    ...keys.map((k) =>
      el('span', { class: 'key' },
        el('i', { class: 'swatch dot', style: `background: var(--cr-series-${slotOf(k.slot, PLOT_PALETTE_SIZE)})` }),
        k.label)))
}
