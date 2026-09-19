import type { Bounds, ChartBlock, GeoMap, Tiles } from '../types'
import { el } from '../dom'
import { svg, n } from '../svg'
import { rampLegend } from '../legend'
import { withTips, type Tips } from '../tip'
import { RAMP_STEPS } from '../palette'

/**
 * A map.
 *
 * No map library, and no tile-loading library either. The server projects
 * every geometry to Web Mercator normalised to the unit square — which is the
 * space an XYZ tile grid is already defined in — so the whole basemap is an
 * `<img>` per tile at a position this file works out with two divisions. A
 * choropleth that pulled in Leaflet would cost more than this entire bundle's
 * budget before drawing anything.
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

  const tips = withTips(panel)
  const { minX, minY, maxX, maxY } = m.bounds
  const canvas = svg('svg', {
    viewBox: `${n(minX)} ${n(minY)} ${n(maxX - minX)} ${n(maxY - minY)}`,
    class: 'geo', part: 'chart', 'aria-hidden': 'true',
  })

  for (const layer of m.layers) draw(layer, m, canvas, tips)

  const stage = el('div', {
    class: 'stage',
    // The box is the data's own aspect ratio, so the map is never letterboxed
    // and never stretched — and the tiles underneath line up either way.
    style: `aspect-ratio: ${(maxX - minX) / (maxY - minY)}`,
  })
  if (m.tiles) stage.append(basemap(m.tiles, m.bounds))
  stage.append(canvas)
  panel.append(stage)

  const key = rampLegend(m.legend)
  if (key && m.layers.includes('polygon')) panel.append(key)
  if (m.tiles) {
    panel.append(el('p', { class: 'credit', part: 'attribution' }, m.tiles.attribution))
  }
  return panel
}

function draw(layer: string, m: GeoMap, canvas: SVGSVGElement, tips: Tips) {
  switch (layer) {
    case 'polygon':
      return polygons(m, canvas, tips)
    case 'heat':
      return heat(m, canvas)
    case 'bubble':
      return dots(m, canvas, tips, true)
    case 'scatter':
      return dots(m, canvas, tips, false)
    case 'flow':
      return flows(m, canvas, tips)
    // A layer this build has never heard of is a normal condition — the server
    // and the viewer ship separately. Drawing the layers it does know beats
    // refusing the whole map.
  }
}

function polygons(m: GeoMap, canvas: SVGSVGElement, tips: Tips) {
  const g = svg('g', { class: 'shapes' })
  for (const s of m.shapes) {
    const path = svg('path', {
      d: s.path, part: 'region',
      fill: `var(--cr-ramp-${Math.min(s.step, RAMP_STEPS - 1) + 1})`,
    })
    tips.bind(path, s.label, s.formatted)
    g.append(path)
  }
  canvas.append(g)
}

/**
 * The density layer: every point blurred into its neighbours.
 *
 * A Gaussian blur over weighted circles rather than a kernel evaluated per
 * pixel. The browser does the same arithmetic on the GPU, and the alternative
 * is a per-pixel loop on the main thread of our customer's customer's page.
 */
function heat(m: GeoMap, canvas: SVGSVGElement) {
  const id = `h${++filters}`
  const span = Number(canvas.getAttribute('viewBox')?.split(' ')[2] ?? 1)

  canvas.append(svg('filter', { id, x: '-20%', y: '-20%', width: '140%', height: '140%' },
    svg('feGaussianBlur', { stdDeviation: n(span * 0.03) })))

  const g = svg('g', { class: 'heat', filter: `url(#${id})` })
  for (const p of m.markers) {
    g.append(svg('circle', {
      cx: n(p.x), cy: n(p.y), r: n(span * 0.035),
      // Opacity and not radius carries the weight. A radius that shrank with
      // the value would say a quiet place is a small place, and the two are
      // different claims.
      fill: `var(--cr-ramp-${RAMP_STEPS})`,
      opacity: n(0.12 + p.weight * 0.5),
    }))
  }
  canvas.append(g)
}

function dots(m: GeoMap, canvas: SVGSVGElement, tips: Tips, sized: boolean) {
  const span = Number(canvas.getAttribute('viewBox')?.split(' ')[2] ?? 1)
  const g = svg('g', { class: 'dots' })
  for (const p of m.markers) {
    // Area, not radius — see scatter.ts. The floor keeps a near-zero value
    // visible, because a marker that renders as nothing is indistinguishable
    // from a row that was filtered away.
    const r = sized ? span * (0.008 + Math.sqrt(p.weight) * 0.022) : span * 0.009
    const mark = svg('circle', { cx: n(p.x), cy: n(p.y), r: n(r), class: 'pin', part: 'marker' })
    tips.bind(mark, p.label, p.size ? `${p.formatted} · ${p.size}` : p.formatted)
    g.append(mark)
  }
  canvas.append(g)
}

/**
 * Flows, as arcs rather than straight lines.
 *
 * Two depots that trade in both directions produce two segments on exactly the
 * same line, and one hides the other. Bowing each one to the left of its own
 * direction of travel separates them, and makes the direction readable without
 * an arrowhead at every scale.
 */
function flows(m: GeoMap, canvas: SVGSVGElement, tips: Tips) {
  const span = Number(canvas.getAttribute('viewBox')?.split(' ')[2] ?? 1)
  const g = svg('g', { class: 'flows' })
  for (const a of m.arcs) {
    const dx = a.x2 - a.x1
    const dy = a.y2 - a.y1
    const bow = 0.18
    const cx = (a.x1 + a.x2) / 2 - dy * bow
    const cy = (a.y1 + a.y2) / 2 + dx * bow
    const arc = svg('path', {
      d: `M${n(a.x1)} ${n(a.y1)}Q${n(cx)} ${n(cy)} ${n(a.x2)} ${n(a.y2)}`,
      class: 'flow', part: 'flow',
      'stroke-width': n(span * (0.002 + a.weight * 0.006)),
    })
    tips.bind(arc, a.label, a.formatted)
    g.append(arc)
  }
  canvas.append(g)
}

let filters = 0

/**
 * The tile layer.
 *
 * Sized once the element knows how wide it is, because the zoom to request is
 * a function of pixels per world unit and nothing else — asking for z=12 in a
 * 200px panel fetches ninety tiles to draw a thumbnail.
 */
function basemap(tiles: Tiles, bounds: Bounds): HTMLElement {
  const layer = el('div', { class: 'tiles', part: 'basemap' })

  const lay = (width: number) => {
    const span = Math.max(bounds.maxX - bounds.minX, 1e-9)
    // 2^z tiles across the world, 256 px each: pick the zoom whose tiles land
    // nearest to their native size, so the basemap is neither blurry nor
    // fetched at four times the detail anybody can see.
    const z = Math.max(0, Math.min(tiles.maxZoom,
      Math.floor(Math.log2(Math.max(width, 1) / (span * 256)))))
    const count = 2 ** z
    const size = 1 / count

    const nodes: HTMLElement[] = []
    for (let tx = Math.floor(bounds.minX * count); tx <= Math.floor(bounds.maxX * count); tx++) {
      for (let ty = Math.floor(bounds.minY * count); ty <= Math.floor(bounds.maxY * count); ty++) {
        // A tile index outside the grid is a request the tile server answers
        // with a 404 and a log line naming our customer.
        if (tx < 0 || ty < 0 || tx >= count || ty >= count) continue
        nodes.push(el('img', {
          src: tiles.url.replace('{z}', String(z)).replace('{x}', String(tx)).replace('{y}', String(ty)),
          alt: '', loading: 'lazy', decoding: 'async',
          // The host page's URL is not the tile server's business. It is our
          // customer's application, and often its path names their customer.
          referrerpolicy: 'no-referrer',
          style: place(tx * size, ty * size, size, bounds),
        }))
      }
    }
    layer.replaceChildren(...nodes)
  }

  // Observed rather than measured once: the panel has no width until it is in
  // the document, and a host page that opens a drawer or rotates a phone
  // changes it afterwards.
  new ResizeObserver((entries) => {
    const box = entries[0]?.contentRect
    if (box) lay(box.width)
  }).observe(layer)
  return layer
}

/** One tile's box, as a percentage of the bounds the SVG is drawn in. */
function place(x: number, y: number, size: number, b: Bounds): string {
  const w = b.maxX - b.minX
  const h = b.maxY - b.minY
  return `left:${((x - b.minX) / w) * 100}%;top:${((y - b.minY) / h) * 100}%;` +
    `width:${(size / w) * 100}%;height:${(size / h) * 100}%`
}
