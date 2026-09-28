import { describe, expect, test } from 'bun:test'
import { anchorOf, namesOf } from '../src/map/labels'
import type { GeoMap } from '../src/types'

/*
 * Where a map's names go. A region's name has to land inside the region, or
 * it names the one beside it; a crowd of places has no name at all.
 */

describe("a region's name", () => {
  test('sits in the middle of a plain one', () => {
    expect(anchorOf('M0 0L4 0L4 2L0 2Z')).toEqual([2, 1])
  })

  test('of several rings, in the largest — the mainland, not an island', () => {
    const [x, y] = anchorOf('M10 10L11 10L11 11L10 11ZM0 0L4 0L4 4L0 4Z')!
    expect(x).toBeCloseTo(2, 9)
    expect(y).toBeCloseTo(2, 9)
  })

  // A C whose centroid is in its mouth: the name moves to the widest stretch
  // of the region itself at that height.
  test('inside a region shaped so its middle is outside it', () => {
    const c = 'M0 0L6 0L6 1L1 1L1 5L6 5L6 6L0 6Z'
    const [x, y] = anchorOf(c)!
    const inside = (x > 0 && x < 1 && y > 0 && y < 6) || (y > 0 && y < 1) || (y > 5 && y < 6)
    expect(inside).toBe(true)
  })
})

describe('what a map names', () => {
  const base = {
    bounds: { minX: 0, minY: 0, maxX: 1, maxY: 1 }, arcs: [], legend: [],
    shapes: [{ label: 'North', path: 'M0 0L1 0L1 1L0 1Z', value: 5, formatted: '5', step: 0 }],
    markers: [{ label: 'Depot', x: 0.5, y: 0.5, value: 9, formatted: '9', weight: 1 }],
  }

  test('its regions and its places, each on the layers that draw them', () => {
    const named = namesOf({ ...base, layers: ['polygon', 'scatter'] } as GeoMap)
    expect(named.map((n) => [n.text, n.beside])).toEqual([['North', false], ['Depot', true]])
    expect(namesOf({ ...base, layers: ['hexbin'] } as GeoMap)).toEqual([])
  })

  // A zone and its depot share a name, and the map says it once.
  test('a name once, though a region and its place both carry it', () => {
    const shared = { ...base, markers: [{ ...base.markers[0]!, label: 'North' }] }
    expect(namesOf({ ...shared, layers: ['polygon', 'scatter'] } as GeoMap).map((n) => n.text)).toEqual(['North'])
  })

  // Beside a bubble, clear of it: its radius, not a dot's.
  test('beside a bubble, past its edge', () => {
    const [depot] = namesOf({ ...base, layers: ['bubble'] } as GeoMap)
    expect(depot?.gap).toBe(4 + 14 + 3)
  })

  // A large map's cell of several places is a crowd, and a crowd has no name.
  test('on a large map, only the cells that are one place', () => {
    const named = namesOf({
      ...base, layers: ['scatter'], markers: [],
      cells: { size: 0.01, x: [0.1, 0.2], y: [0.1, 0.2], n: [1, 7], v: [3, 40], l: ['Alone', ''] },
    } as unknown as GeoMap)
    expect(named.map((n) => n.text)).toEqual(['Alone'])
  })
})
