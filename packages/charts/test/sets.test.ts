import { describe, expect, test } from 'bun:test'
import { boxOf, describeArea, pickOf, world } from '../src/map/sets'
import { latitude, longitude } from '../src/map/geo'
import type { GeoMap } from '../src/types'

/*
 * What a map sets on its report. The numbers reach the server as a filter,
 * which refuses a longitude past 180 and a south north of its north — so every
 * view a reader can reach has to be a box the server takes, and the box has
 * to be the one on the screen.
 */

describe('the box a view is', () => {
  test('is the degrees at its edges', () => {
    const [x0, y0] = world(52.4, 4.8)
    const [x1, y1] = world(52.3, 5.0)
    expect(boxOf({ x: x0, y: y0, w: x1 - x0, h: y1 - y0 })).toEqual([52.3, 4.8, 52.4, 5.0])
  })

  test('the projection is the one the viewer reads places back in', () => {
    for (const [lat, lon] of [[0, 0], [52.37, 4.9], [-33.87, 151.21], [64.1, -21.9]] as const) {
      const [x, y] = world(lat, lon)
      expect(latitude(y)).toBeCloseTo(lat, 9)
      expect(longitude(x)).toBeCloseTo(lon, 9)
    }
  })

  // Zoomed out past the world's edges: a longitude of -250 is a box the
  // server refuses, where the reader asked for everywhere.
  test('a view wider than the world is all of it', () => {
    const [south, west, north, east] = boxOf({ x: -0.4, y: -0.2, w: 1.8, h: 1.4 })
    expect([west, east]).toEqual([-180, 180])
    expect(south).toBeCloseTo(-85.05113, 4)
    expect(north).toBeCloseTo(85.05113, 4)
  })

  test('a view over the antimeridian comes back west of east', () => {
    // From 170° east across the date line to 170° west, around Fiji.
    const [x0, y0] = world(-10, 170)
    const [, y1] = world(-25, 170)
    const [south, west, north, east] = boxOf({ x: x0, y: y0, w: 20 / 360, h: y1 - y0 })
    expect(west).toBeCloseTo(170, 4)
    expect(east).toBeCloseTo(-170, 4)
    expect(south).toBeCloseTo(-25, 4)
    expect(north).toBeCloseTo(-10, 4)
  })
})

describe('a click on a place', () => {
  const on = (values?: string[]) => ({ pick: { filter: 'depot', values } }) as unknown as GeoMap
  const clicks = (m: GeoMap, label: string) => {
    const sent: unknown[] = []
    pickOf(m, (name, value) => sent.push([name, value]))?.toggle(label)
    return sent
  }

  test('narrows the report to that place', () => {
    expect(clicks(on(), 'North')).toEqual([['depot', { op: 'in', values: ['North'] }]])
  })

  test('on the place it is narrowed to lets it go', () => {
    expect(clicks(on(['North']), 'North')).toEqual([['depot', null]])
  })

  // Several at once is the filter bar's: a map is for pointing at one place,
  // and a click that added to a list would need a second gesture to undo.
  test('on another place moves to it', () => {
    expect(clicks(on(['North']), 'South')).toEqual([['depot', { op: 'in', values: ['South'] }]])
    expect(clicks(on(['North', 'East']), 'North')).toEqual([['depot', { op: 'in', values: ['North'] }]])
  })

  test('is nothing on a map that sets no filter, or a host that takes none', () => {
    expect(pickOf({} as GeoMap, () => {})).toBeUndefined()
    expect(pickOf(on(), undefined)).toBeUndefined()
    expect(pickOf(on(['North']), () => {})?.picked('North')).toBe(true)
  })
})

// Said the way a person says a place: a hemisphere, not a sign. "-5.2° east"
// is a longitude nobody reads as five degrees west.
describe('an area in words', () => {
  test('a box names its hemispheres', () => {
    expect(describeArea({ op: 'within', values: [50.1, -5.2, 52.345, 1.3] }))
      .toBe('50.1°N to 52.35°N, 5.2°W to 1.3°E')
    expect(describeArea({ op: 'within', values: [-34.2, 150.9, -33.5, 151.4] }))
      .toBe('34.2°S to 33.5°S, 150.9°E to 151.4°E')
  })

  test('a distance names its place', () => {
    expect(describeArea({ op: 'near', values: [52.37, 4.9, 10] })).toBe('Within 10 km of 52.37°N, 4.9°E')
  })

  test('nothing set, or not an area, is nothing', () => {
    expect(describeArea(undefined)).toBeNull()
    expect(describeArea({ op: 'within', values: ['x', 1, 2, 3] })).toBeNull()
    expect(describeArea({ op: 'in', values: [1] })).toBeNull()
  })
})
