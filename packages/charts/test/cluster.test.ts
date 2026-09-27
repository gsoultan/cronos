import { describe, expect, test } from 'bun:test'
import { cluster, count } from '../src/map/cluster'
import type { Marker } from '../src/types'

const at = (label: string, x: number, y: number): Marker =>
  ({ label, x, y, value: 1, formatted: '1', weight: 1 })

describe('clustering', () => {
  test('gathers points closer than a cell and leaves the rest alone', () => {
    const points = [at('a', 0.5, 0.5), at('b', 0.50001, 0.5), at('c', 0.5, 0.50001), at('far', 0.6, 0.6)]
    const groups = cluster(points, 0.001)
    expect(groups).toHaveLength(2)
    const big = groups.find((g) => g.members.length === 3)!
    expect(big.members.map((m) => m.label).sort()).toEqual(['a', 'b', 'c'])
    // Drawn at the middle of its members.
    expect(big.x).toBeCloseTo((0.5 + 0.50001 + 0.5) / 3, 12)
  })

  test('joins across a grid line — a cell edge is not a boundary on the map', () => {
    const groups = cluster([at('left', 0.00999, 0.5), at('right', 0.01001, 0.5)], 0.01)
    expect(groups).toHaveLength(1)
  })

  test('groups the same points the same way in whatever order they arrive', () => {
    const points = Array.from({ length: 200 }, (_, i) =>
      at(`p${i}`, 0.5 + ((i * 37) % 101) * 0.0001, 0.3 + ((i * 53) % 97) * 0.0001))
    const shape = (ps: Marker[]) => cluster(ps, 0.0012)
      .map((g) => g.members.map((m) => m.label).sort().join())
      .sort()
    expect(shape([...points].reverse())).toEqual(shape(points))
  })

  test('every point is in exactly one cluster', () => {
    const points = Array.from({ length: 500 }, (_, i) => at(`p${i}`, (i % 23) * 0.001, (i % 19) * 0.001))
    const seen = cluster(points, 0.0025).flatMap((g) => g.members.map((m) => m.label))
    expect(seen.sort()).toEqual(points.map((p) => p.label).sort())
  })
})

describe('the count in a cluster', () => {
  test('is short enough for the circle it is written in', () => {
    expect(count(7)).toBe('7')
    expect(count(999)).toBe('999')
    expect(count(1250)).toBe('1.3k')
    expect(count(48_000)).toBe('48k')
  })
})
