import type { DrawOptions, GeoMap, MapArea } from '../types'
import { el } from '../dom'
import { svg } from '../svg'
import { g, latitude, longitude } from './geo'
import type { View, Viewport } from './view'

/** Clicking a label on a map: whether it is picked now, and picking it. */
export interface Pick {
  picked(label: string): boolean
  toggle(label: string): void
}

type SetFilter = NonNullable<DrawOptions['filter']>

/**
 * The report filter a click on this map sets, as something a layer can call —
 * or nothing, where the map sets none or the host takes no filters.
 *
 * A click on the one label the filter holds lets it go; a click on any other
 * narrows to that one. Several at once is the filter bar's business: a map is
 * for pointing at a place.
 */
export function pickOf(m: GeoMap, set: SetFilter | undefined): Pick | undefined {
  const p = m.pick
  if (!p || !set) return undefined
  const now = p.values ?? []
  return {
    picked: (label) => now.includes(label),
    toggle: (label) => set(p.filter, now.length === 1 && now[0] === label ? null
      : { op: 'in', values: [label] }),
  }
}

/**
 * What a map offers when the report has an area filter over its coordinates:
 * "Filter to this view", which narrows the report to the box the reader has
 * zoomed to, and — once it is set — "Show everywhere", which lets it go. The
 * area it holds is outlined on the map, so a reader sees what the rest of the
 * report is narrowed to.
 */
export function areaTools(stage: HTMLElement, canvas: SVGElement, port: Viewport,
  area: MapArea | undefined, set: SetFilter | undefined) {
  if (!area || !set) return
  const tools = el('div', { class: 'geo-tools', part: 'map-tools' })
  const button = (text: string, act: () => void) => {
    const b = el('button', { type: 'button' }, text)
    // The press is the button's, not the start of a drag on the map under it.
    b.addEventListener('pointerdown', (e) => e.stopPropagation())
    b.addEventListener('click', act)
    tools.append(b)
  }
  button('Filter to this view', () => set(area.filter, { op: 'within', values: boxOf(port.view()) }))
  if (area.op) button('Show everywhere', () => set(area.filter, null))
  stage.append(tools)
  const outline = shapeOf(area)
  if (outline) canvas.append(outline)
}

/**
 * The part of the world a view shows, as an area filter's numbers: south,
 * west, north, east.
 *
 * A view wider than the world is all of it, and one over the antimeridian
 * comes back west of east — which the server reads as a box across it, rather
 * than as a longitude past 180 it would refuse.
 */
export function boxOf(v: View): number[] {
  const r = (n: number) => Math.round(n * 1e5) / 1e5
  const wrap = (lon: number) => (lon < -180 ? lon + 360 : lon > 180 ? lon - 360 : lon)
  const whole = v.w >= 1
  const west = whole ? -180 : wrap(longitude(v.x))
  const east = whole ? 180 : wrap(longitude(v.x + v.w))
  return [r(latitude(Math.min(v.y + v.h, 1))), r(west), r(latitude(Math.max(v.y, 0))), r(east)]
}

/** The area a report is narrowed to, in world units: a box, or a circle. */
function shapeOf(a: MapArea): SVGElement | null {
  const v = a.values ?? []
  if (a.op === 'within' && v.length === 4) {
    const [s = 0, w = 0, n = 0, e = 0] = v
    const [x0, y0] = world(n, w)
    const [x1, y1] = world(s, e)
    const box = (left: number, right: number) =>
      `M${g(left)} ${g(y0)}H${g(right)}V${g(y1)}H${g(left)}Z`
    // West of east is a box across the antimeridian: both ends of the world.
    return svg('path', { class: 'geo-area', part: 'map-area',
      d: x0 <= x1 ? box(x0, x1) : box(x0, 1) + box(0, x1) })
  }
  if (a.op === 'near' && v.length === 3) {
    const [lat = 0, lon = 0, km = 0] = v
    const pts = Array.from({ length: 65 }, (_, i) => {
      const [la, lo] = destination(lat, lon, km, (i / 64) * 360)
      // On the centre's side of the antimeridian, so a circle over it is a
      // circle rather than a line across the world.
      return world(la, lo + 360 * Math.round((lon - lo) / 360))
    })
    return svg('path', { class: 'geo-area', part: 'map-area',
      d: `M${pts.map(([x, y]) => `${g(x)} ${g(y)}`).join('L')}Z` })
  }
  return null
}

/**
 * An area filter's value in words — "52.3°N to 52.4°N, 5.2°W to 1.3°E", or
 * "Within 10 km of 52.37°N, 4.9°E" — or nothing, for one that is not set or
 * not an area. One wording, here, so the embed's bar and the portal's say the
 * same thing about the same filter.
 */
export function describeArea(value: { op: string; values: unknown[] } | null | undefined): string | null {
  const v = (value?.values ?? []).map(Number)
  if (!value || v.some((n) => !Number.isFinite(n))) return null
  const r = (n: number) => Math.round(Math.abs(n) * 100) / 100
  const lat = (n: number) => `${r(n)}°${n < 0 ? 'S' : 'N'}`
  const lon = (n: number) => `${r(n)}°${n < 0 ? 'W' : 'E'}`
  const [a = 0, b = 0, c = 0, d = 0] = v
  if (value.op === 'near' && v.length === 3) return `Within ${Math.round(c * 10) / 10} km of ${lat(a)}, ${lon(b)}`
  if (value.op === 'within' && v.length === 4) return `${lat(a)} to ${lat(c)}, ${lon(b)} to ${lon(d)}`
  return null
}

/** A coordinate in the world units the server projects into — its project. */
export function world(lat: number, lon: number): [number, number] {
  const phi = (Math.max(-85.05112878, Math.min(85.05112878, lat)) * Math.PI) / 180
  return [(lon + 180) / 360, 0.5 - Math.log(Math.tan(phi) + 1 / Math.cos(phi)) / (2 * Math.PI)]
}

/** Where km along a bearing from a place lands, on a sphere. */
function destination(lat: number, lon: number, km: number, bearing: number): [number, number] {
  const r = Math.PI / 180
  const d = km / 6371
  const phi = lat * r
  const theta = bearing * r
  const to = Math.asin(Math.sin(phi) * Math.cos(d) + Math.cos(phi) * Math.sin(d) * Math.cos(theta))
  const along = lon * r + Math.atan2(Math.sin(theta) * Math.sin(d) * Math.cos(phi),
    Math.cos(d) - Math.sin(phi) * Math.sin(to))
  return [to / r, ((along / r + 540) % 360) - 180]
}
