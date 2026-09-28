import { referenced, referencedNames } from './secrets'
import type { BasemapProvider, Tile, TileMap } from './types'

/**
 * What a map block may combine, restated from the server for the inspector.
 *
 * definitions.ts leaves validation to the server, which compiles every block
 * before it stores anything, and it is right to: a second copy of a rule is a
 * second thing to keep in step. These are the exception because they decide
 * which controls the inspector offers at all. A builder that offers hexagons
 * over shaded regions and then refuses the save teaches the rules one refusal
 * at a time; one that does not offer them has nothing to refuse.
 *
 * The server keeps the last word — mapspec.go, maplayer.go, basemap.go and
 * reportvalidate.go in internal/core/definition. If a rule here drifts from
 * those, the cost is a save refused with a sentence naming the rule, never a
 * definition stored that means something else.
 */

/** The layers that draw the geometry field rather than a coordinate per row. */
const SHAPED = ['polygon', 'line']

/**
 * The layers that already spend colour on the value: the three shaded from
 * the ramp, and a heat field, whose intensity is the value.
 */
const COLOURED = new Set(['polygon', 'line', 'hexbin', 'heat'])

/** The layers shaded from the ramp, which classes and a diverging ramp colour. */
const SHADED = new Set(['polygon', 'line', 'hexbin'])

/** Whether any layer is shaded from the ramp. */
export const shades = (layers: string[]) => layers.some((l) => SHADED.has(l))

/** As wide as a hexagon may be. Wider than a continent is one hexagon. */
export const MAX_HEX_KM = 5000

/**
 * What the server will draw: the layers named, or what the fields imply when
 * none are — shapes for a geometry field, dots for coordinates.
 *
 * The rules below are asked of this rather than of the picker's value,
 * because this is what the server checks them against.
 */
export function drawnLayers(map: TileMap): string[] {
  if (map.layers && map.layers.length > 0) return map.layers
  if (map.geometry) return ['polygon']
  if (map.lat && map.lon) return ['scatter']
  return []
}

/** Whether any layer reads the geometry field. */
export const readsShapes = (layers: string[]) => layers.some((l) => SHAPED.includes(l))

/** Whether any layer reads a latitude and a longitude. Hexagons and clusters
 *  do, because both are made of the points inside them. */
export const readsPoints = (layers: string[]) => layers.some((l) => !SHAPED.includes(l))

/** Whether a layer shows the value by its colour. */
export const colours = (layer: string) => COLOURED.has(layer)

/**
 * Whether points may be coloured by a category.
 *
 * Only when nothing drawn already colours by the value. Colour is one
 * channel, and dots coloured by carrier over regions shaded by revenue would
 * give it two jobs under one legend.
 */
export function takesSeries(layers: string[]): boolean {
  return layers.length > 0 && !layers.some(colours)
}

/**
 * The layers that cannot join the ones chosen.
 *
 * Hexagons against regions or routes, either way round. All three shade from
 * the ramp, but from different rows — a region's value is its own row and a
 * hexagon's is the points inside it — and one legend cannot explain two
 * scales.
 */
export function excludedBy(layers: string[]): string[] {
  if (layers.includes('hexbin')) return SHAPED
  return readsShapes(layers) ? ['hexbin'] : []
}

/**
 * A map block given new layers, less what they no longer allow.
 *
 * Dropped in the same change rather than left for the save to refuse, the way
 * clearing a split drops the stacking: the server refuses a series beside a
 * layer that colours by value and a hexagon width with no hexagons, and the
 * control for either has left the screen by then, so the author could not see
 * what to fix.
 */
export function relayer(
  block: Pick<Tile, 'map' | 'series'>, layers: string[],
): Pick<Tile, 'map' | 'series'> {
  const map: TileMap = {
    ...block.map,
    layers,
    hexKm: layers.includes('hexbin') ? block.map?.hexKm : undefined,
  }
  // How the shades are classed means nothing once nothing is shaded, and the
  // server refuses it rather than ignore it.
  if (!shades(drawnLayers(map))) {
    map.classify = map.breaks = map.ramp = map.midpoint = undefined
  }
  return { map, series: takesSeries(drawnLayers(map)) ? block.series : undefined }
}

/** What the basemap picker offers: nothing, a named provider, or a url. */
export type BasemapChoice = 'none' | 'url' | BasemapProvider

/** Which of them a map has now. */
export function basemapChoice(map: TileMap): BasemapChoice {
  if (map.provider) return map.provider
  return map.basemap === undefined ? 'none' : 'url'
}

/**
 * Everything a change of basemap sets, including what it clears.
 *
 * Nothing carries across. The server refuses a url beside a provider, a style
 * or a key on a url, and a language anywhere but Google — and a key is one
 * company's, so carried to another provider it would name the wrong account.
 * A zoom cap goes too: it was chosen for the tiles it capped.
 *
 * The url starts empty rather than absent, which is what keeps its field on
 * screen while it is typed. Absent is no basemap at all.
 */
export function switchBasemap(choice: BasemapChoice): Partial<TileMap> {
  return {
    provider: choice === 'none' || choice === 'url' ? undefined : choice,
    style: undefined,
    key: undefined,
    language: undefined,
    basemapRegion: undefined,
    maxZoom: undefined,
    basemap: choice === 'url' ? '' : undefined,
    attribution: undefined,
  }
}

interface Option { value: string; label: string }

/** Each provider's styles, default first, as the server lists them. */
const STYLES: Record<BasemapProvider, Option[]> = {
  openstreetmap: [{ value: 'standard', label: 'Standard' }],
  mapbox: [
    { value: 'streets', label: 'Streets' },
    { value: 'outdoors', label: 'Outdoors' },
    { value: 'light', label: 'Light' },
    { value: 'dark', label: 'Dark' },
    { value: 'satellite', label: 'Satellite' },
    { value: 'satellite-streets', label: 'Satellite with streets' },
    { value: 'navigation-day', label: 'Navigation, day' },
    { value: 'navigation-night', label: 'Navigation, night' },
  ],
  google: [
    { value: 'roadmap', label: 'Road map' },
    { value: 'satellite', label: 'Satellite' },
    { value: 'terrain', label: 'Terrain' },
    { value: 'hybrid', label: 'Satellite with labels' },
  ],
}

/** What a provider draws when the file names no style. */
export function defaultStyle(provider: BasemapProvider): string | undefined {
  return STYLES[provider]?.[0]?.value
}

/**
 * The styles to offer for a provider.
 *
 * The one in the file is offered too when it is not a named one, which at
 * Mapbox is a style somebody made in Studio, `owner/style`. Left out, the
 * picker would show a blank for the style the map is drawn in — which reads
 * as no style at all, and invites picking one over it.
 */
export function styleOptions(provider: BasemapProvider, current?: string): Option[] {
  const named = STYLES[provider] ?? []
  if (!current || named.some((s) => s.value === current)) return named
  return [...named, { value: current, label: provider === 'mapbox' ? `${current} (Studio)` : current }]
}

/**
 * The secret a provider reads its key from when a map names none —
 * CRONOS_SECRET_MAPBOX_TOKEN and CRONOS_SECRET_GOOGLE_MAPS_KEY in an
 * environment. One per provider, because a deployment has one Mapbox account
 * far more often than it has several. basemapprovider.go's DefaultKey.
 */
export const DEFAULT_KEY: Partial<Record<BasemapProvider, string>> = {
  mapbox: 'mapbox-token',
  google: 'google-maps-key',
}

/** How the secret behind a tile url's key has to be named. */
export const TILE_KEY_PREFIX = 'tiles-'

/** A secret a basemap sends to every reader's browser. */
export interface BasemapKey {
  /** The secret's name — or, when the file's key is not one reference, the key as written. */
  name: string
  /** What the name has to start with for the server to send it anywhere. */
  prefix: string
  /** Whether it does: one whole reference, with that prefix. */
  sendable: boolean
}

/**
 * The secrets a basemap reads, and whether the server will send each one.
 *
 * A tile key is public by construction — it is in every tile request, and a
 * reader who opens the network tab has it — so the server sends a secret to a
 * browser only when its name says it is a key: `mapbox-…`, `google-…`, or
 * `tiles-…` in a url. Without that rule `${secret:warehouse-password}` in a
 * tile url would send the database password to every reader. The server
 * refuses the rest on save and again at render; the builder refuses to store
 * a value under them, which is the one place it could otherwise help leak one.
 */
export function basemapKeys(map: TileMap): BasemapKey[] {
  const fallback = map.provider ? DEFAULT_KEY[map.provider] : undefined
  if (map.provider && fallback) {
    const prefix = `${map.provider}-`
    const name = map.key ? referenced(map.key) : fallback
    return [{ name: name ?? map.key ?? '', prefix, sendable: !!name?.startsWith(prefix) }]
  }
  if (map.provider || !map.basemap) return []
  return referencedNames(map.basemap).map((name) => ({
    name, prefix: TILE_KEY_PREFIX, sendable: name.startsWith(TILE_KEY_PREFIX),
  }))
}

/**
 * Why a value cannot be a map's key, before it is stored.
 *
 * Mapbox issues two kinds of token and only one belongs in a browser: a secret
 * token (sk.) can change the account. The server will not send one — the map
 * draws without its basemap and the log says why — so storing one would be a
 * key that looks set and never works, and a secret token sitting in a place
 * made for public ones.
 */
export function keyValueProblem(secretName: string, value: string): string | undefined {
  if (secretName.startsWith('mapbox-') && value.trim().startsWith('sk.')) {
    return 'That is a Mapbox secret token (sk.), which can change the account — and a map key '
      + 'reaches every reader’s browser. Use a public token (pk.) with the styles:tiles scope.'
  }
  return undefined
}
