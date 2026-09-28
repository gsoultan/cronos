import { expect, test } from 'bun:test'
import {
  basemapChoice, basemapKeys, detectMap, drawnLayers, excludedBy, keyValueProblem, readsPoints, readsShapes,
  plays, relayer, studioStyle, styleOptions, switchBasemap, takesSeries,
} from './maps'
import type { Field, TileMap } from './types'

/* Colour is one channel. The server refuses a series on a map that already
   spends it on the value, so the inspector must not offer one there. */
test('only a map that leaves colour free takes a series', () => {
  expect(takesSeries(['scatter', 'bubble', 'cluster', 'flow'])).toBe(true)
  for (const coloured of ['polygon', 'line', 'hexbin', 'heat']) {
    expect(takesSeries(['scatter', coloured])).toBe(false)
  }
  // A map that draws nothing has nothing to colour.
  expect(takesSeries([])).toBe(false)
})

test('hexagons and shapes exclude each other, either way round', () => {
  expect(excludedBy(['hexbin', 'scatter'])).toEqual(['polygon', 'line', 'h3'])
  expect(excludedBy(['polygon'])).toEqual(['hexbin', 'h3'])
  expect(excludedBy(['line', 'flow'])).toEqual(['hexbin', 'h3'])
  // H3 cells shade from the ramp too, from rows of their own: alone, as hexagons are.
  expect(excludedBy(['h3', 'scatter'])).toEqual(['polygon', 'line', 'hexbin'])
  expect(excludedBy(['heat', 'cluster'])).toEqual([])
})

/* The server infers a layer when none is named, and checks its rules against
   that — so a map with a shapes field and no layers is shaded regions, and
   refuses a series exactly as if the author had picked them. */
test('a map with no layers draws what its fields imply', () => {
  expect(drawnLayers({ layers: ['heat'], geometry: 'shape' })).toEqual(['heat'])
  expect(drawnLayers({ geometry: 'shape', lat: 'lat', lon: 'lon' })).toEqual(['polygon'])
  expect(drawnLayers({ layers: [], lat: 'lat', lon: 'lon' })).toEqual(['scatter'])
  expect(drawnLayers({ lat: 'lat' })).toEqual([])
})

test('hexagons and clusters read coordinates, regions and routes read shapes', () => {
  expect(readsPoints(['hexbin'])).toBe(true)
  expect(readsPoints(['cluster'])).toBe(true)
  expect(readsPoints(['polygon', 'line'])).toBe(false)
  // H3 cells bin places, unless they are a warehouse's own cells.
  expect(readsPoints(['h3'])).toBe(true)
  expect(readsPoints(['h3'], 'cell')).toBe(false)
  expect(readsShapes(['line'])).toBe(true)
  expect(readsShapes(['hexbin', 'heat'])).toBe(false)
})

/* The way clearing a split drops the stacking: a setting kept for a layer that
   is gone is refused on save, by which time its control has left the screen. */
test('a layer that colours by value drops the series in the same change', () => {
  const next = relayer({ map: { layers: ['scatter'] }, series: 'carrier' }, ['scatter', 'heat'])
  expect(next.series).toBeUndefined()
  expect(next.map?.layers).toEqual(['scatter', 'heat'])

  const kept = relayer({ map: { layers: ['scatter'] }, series: 'carrier' }, ['scatter', 'cluster'])
  expect(kept.series).toBe('carrier')

  // No layers and a shapes field is shaded regions, which colour by value too.
  const cleared = relayer({ map: { layers: ['scatter'], geometry: 'shape' }, series: 'carrier' }, [])
  expect(cleared.series).toBeUndefined()
})

// The server refuses a map that plays through time beside hexagons or H3
// cells, and the control has gone from the screen by then.
test('playing through time goes when hexagons or cells arrive', () => {
  const timed: TileMap = { layers: ['scatter'], lat: 'lat', lon: 'lon', time: 'day', timeGrain: 'week' }
  for (const folded of ['hexbin', 'h3']) {
    const next = relayer({ map: timed }, ['scatter', folded]).map
    expect(next?.time).toBeUndefined()
    expect(next?.timeGrain).toBeUndefined()
  }
  expect(relayer({ map: timed }, ['scatter', 'heat']).map).toMatchObject({ time: 'day', timeGrain: 'week' })
  expect(plays(['polygon', 'line', 'flow'])).toBe(true)
  expect(plays([])).toBe(false)
})

test('a hexagon width goes with the last hexbin layer', () => {
  expect(relayer({ map: { layers: ['hexbin'], hexKm: 5 } }, ['scatter']).map?.hexKm).toBeUndefined()
  expect(relayer({ map: { layers: ['hexbin'], hexKm: 5 } }, ['hexbin', 'cluster']).map?.hexKm).toBe(5)
})

/* Each form's settings mean nothing to the other's, and the server refuses
   every mix: a url beside a provider, a style or a key on a url. */
test('switching basemap leaves nothing of the old one behind', () => {
  const custom: TileMap = {
    basemap: 'https://tile.example.org/{z}/{x}/{y}.png', attribution: '© Example', maxZoom: 18,
  }
  const mapbox = { ...custom, ...switchBasemap('mapbox') }
  expect(basemapChoice(mapbox)).toBe('mapbox')
  expect(mapbox.basemap).toBeUndefined()
  expect(mapbox.attribution).toBeUndefined()
  expect(mapbox.maxZoom).toBeUndefined()

  const google: TileMap = {
    provider: 'google', style: 'satellite', key: '${secret:google-maps-acme}',
    language: 'id', basemapRegion: 'ID', maxZoom: 16,
  }
  const url = { ...google, ...switchBasemap('url') }
  expect(basemapChoice(url)).toBe('url')
  expect(url).toMatchObject({ basemap: '' })
  for (const left of ['provider', 'style', 'key', 'language', 'basemapRegion', 'maxZoom'] as const) {
    expect(url[left]).toBeUndefined()
  }

  // A key is one company's. Carried to another provider it names the wrong
  // account, and Google's label settings mean nothing at Mapbox.
  const moved = { ...google, ...switchBasemap('mapbox') }
  expect(moved.key).toBeUndefined()
  expect(moved.language).toBeUndefined()
  expect(moved.basemapRegion).toBeUndefined()

  expect(basemapChoice({ ...google, ...switchBasemap('none') })).toBe('none')
})

test('a Studio style in the file is offered, so it shows and survives', () => {
  const offered = styleOptions('mapbox', 'acme/cjx1abc2de3').map((o) => o.value)
  expect(offered).toContain('acme/cjx1abc2de3')
  // The provider's default stays first, as the server lists it.
  expect(offered[0]).toBe('streets')

  expect(styleOptions('mapbox', 'dark').filter((o) => o.value === 'dark')).toHaveLength(1)
  expect(styleOptions('openstreetmap').map((o) => o.value)).toEqual(['standard'])
  expect(styleOptions('google').map((o) => o.value))
    .toEqual(['roadmap', 'satellite', 'terrain', 'hybrid', 'dark', 'auto'])
})

/*
 * Which secret a map's key is, so the builder can say whether it is set and
 * store one. A provider with no key named in the file reads its default — the
 * name somebody opening Settings has to be told is missing.
 */
test('a map reads its provider’s default key unless the file names one', () => {
  expect(basemapKeys({ provider: 'mapbox' })).toEqual([
    { name: 'mapbox-token', prefix: 'mapbox-', sendable: true },
  ])
  expect(basemapKeys({ provider: 'google', key: '${secret:google-maps-acme}' })).toEqual([
    { name: 'google-maps-acme', prefix: 'google-', sendable: true },
  ])
  // OpenStreetMap takes no key, and a map with no basemap asks nobody.
  expect(basemapKeys({ provider: 'openstreetmap' })).toEqual([])
  expect(basemapKeys({})).toEqual([])
})

/* A tile key reaches every reader's browser, so only a secret whose name says
   it is one may be sent. Anything else is refused here before a value could be
   stored under it — `${secret:warehouse-password}` in a tile url would
   otherwise send the database password to everybody. */
test('only a key named for its provider is sent to a browser', () => {
  expect(basemapKeys({ provider: 'mapbox', key: '${secret:warehouse-password}' })[0])
    .toMatchObject({ name: 'warehouse-password', sendable: false })
  // A key that is not one whole reference is not a secret at all.
  expect(basemapKeys({ provider: 'mapbox', key: 'pk.${secret:mapbox-x}' })[0]?.sendable).toBe(false)
  // Google's prefix is google-, and a Mapbox name is not it.
  expect(basemapKeys({ provider: 'google', key: '${secret:mapbox-token}' })[0]?.sendable).toBe(false)
})

test('a tile url’s keys are every secret it names, tiles- or refused', () => {
  const url = 'https://t.example/{z}/{x}/{y}.png?key=${secret:tiles-acme}&db=${secret:warehouse_password}'
  expect(basemapKeys({ basemap: url })).toEqual([
    { name: 'tiles-acme', prefix: 'tiles-', sendable: true },
    { name: 'warehouse_password', prefix: 'tiles-', sendable: false },
  ])
  expect(basemapKeys({ basemap: 'https://t.example/{z}/{x}/{y}.png' })).toEqual([])
})

/* The server will not send a Mapbox secret token, so one stored as a map key
   looks set and never works — and is a token that can change the account,
   sitting in a place made for public ones. */
test('a Mapbox secret token is refused as a map key', () => {
  expect(keyValueProblem('mapbox-token', 'sk.eyJ1Ijoi')).toBeDefined()
  expect(keyValueProblem('mapbox-token', '  sk.eyJ1Ijoi')).toBeDefined()
  expect(keyValueProblem('mapbox-token', 'pk.eyJ1Ijoi')).toBeUndefined()
  // Not a Mapbox key, so not Mapbox's rule.
  expect(keyValueProblem('google-maps-key', 'sk.whatever')).toBeUndefined()
})

/* A new map starts from what the dataset's names say it holds, hidden fields
   included — coordinates usually are hidden, being no use in a table. */
const field = (name: string, type: Field['type'] = 'decimal', hidden = true): Field =>
  ({ name, label: name, type, role: 'dimension', hidden })

test('a new map reads the coordinates the names give', () => {
  expect(detectMap([field('id', 'string', false), field('lat'), field('lon')]))
    .toMatchObject({ layers: ['scatter'], lat: 'lat', lon: 'lon' })
  expect(detectMap([field('depot_latitude'), field('depot_lng')]))
    .toMatchObject({ layers: ['scatter'], lat: 'depot_latitude', lon: 'depot_lng' })
})

test('and where a flow lands, as a flow', () => {
  const m = detectMap([field('to_lat'), field('to_lon'), field('from_lat'), field('from_lon')])
  expect(m).toMatchObject({ layers: ['flow', 'scatter'], lat: 'from_lat', lon: 'from_lon', toLat: 'to_lat', toLon: 'to_lon' })
})

test('a geometry, as regions or as routes', () => {
  expect(detectMap([field('shape', 'string')])).toMatchObject({ layers: ['polygon'], geometry: 'shape' })
  expect(detectMap([field('path', 'string')])).toMatchObject({ layers: ['line'], geometry: 'path' })
  // Regions with a place in each: a zone and its depot.
  expect(detectMap([field('shape', 'string'), field('lat'), field('lon')]))
    .toMatchObject({ layers: ['polygon', 'scatter'], geometry: 'shape', lat: 'lat', lon: 'lon' })
})

test('nothing recognisable is nothing guessed', () => {
  const m = detectMap([field('region', 'string', false), field('parcels'), field('shape_area')])
  expect(m.layers).toEqual(['scatter'])
  expect(m.lat ?? m.lon ?? m.geometry).toBeUndefined()
  // A latitude without a longitude is not a place.
  expect(detectMap([field('lat')]).lat).toBeUndefined()
  // A number named like a geometry is not one.
  expect(detectMap([field('shape')]).geometry).toBeUndefined()
})

/* A Studio style as Studio hands it over — and never the token its share page
   carries, which would be in the report's file for ever. */
test('a Studio style is read from whatever Studio gave', () => {
  for (const pasted of [
    'mapbox://styles/acme-maps/clx7ab12c00ff01qs',
    'https://api.mapbox.com/styles/v1/acme-maps/clx7ab12c00ff01qs.html?title=view&access_token=pk.eyJ1Ijoi#9/52.3/4.9',
    'https://studio.mapbox.com/styles/acme-maps/clx7ab12c00ff01qs/edit/',
    '  acme-maps/clx7ab12c00ff01qs ',
  ]) expect(studioStyle(pasted)).toBe('acme-maps/clx7ab12c00ff01qs')
  for (const wrong of ['light', 'acme-maps/', 'Acme/style', 'https://example.org/styles/v1/a/b', 'a/b/c'])
    expect(studioStyle(wrong)).toBeUndefined()
})
