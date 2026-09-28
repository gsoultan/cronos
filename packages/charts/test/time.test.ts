import { describe, expect, test } from 'bun:test'
import { frameOf, periodValues } from '../src/map/time'
import type { GeoMap } from '../src/types'

/*
 * A period of a map that plays through time, as the layers are handed it:
 * the same map, each mark with its value then. What the map opens on is every
 * period together, and stays so.
 */

const played: GeoMap = {
  bounds: { minX: 0, minY: 0, maxX: 1, maxY: 1 },
  layers: ['polygon', 'bubble', 'flow'],
  frames: ['1 Aug 2026', '2 Aug 2026', '3 Aug 2026'],
  legend: [{ step: 0, from: '0', to: '30' }],
  frameLegend: [{ step: 0, from: '0', to: '12' }],
  shapes: [{
    label: 'North', path: 'M0 0L1 0L1 1Z', value: 30, formatted: '30', step: 5,
    frames: { 0: { v: 10, f: '10', s: 2 }, 2: { v: 20, f: '20', s: 4 } },
  }],
  lines: [],
  markers: [
    { label: 'A', x: 0.1, y: 0.1, value: 8, formatted: '8', weight: 1, size: '3',
      frames: { 0: { v: 2, f: '2', w: 0.25, z: '2' }, 1: { v: 1, f: '1', w: 0.125, z: '1' }, 2: { v: 5, f: '5', w: 0.625, z: '1' } } },
    { label: 'B', x: 0.2, y: 0.2, value: 12, formatted: '12', weight: 1,
      frames: { 0: { v: 4, f: '4', w: 0.5 }, 2: { v: 8, f: '8', w: 1 } } },
  ],
  arcs: [{ label: 'A', x1: 0.1, y1: 0.1, x2: 0.2, y2: 0.2, value: 8, formatted: '8', weight: 1,
    frames: { 1: { v: 1, f: '1', w: 0.125 } } }],
}

describe('a period', () => {
  test("gives each place its value, size and weight then, and leaves off one with nothing in it", () => {
    const f = frameOf(played, 1)
    expect(f.markers.map((p) => [p.label, p.value, p.formatted, p.weight, p.size])).toEqual([['A', 1, '1', 0.125, '1']])
    expect(f.arcs.map((a) => [a.label, a.value, a.weight])).toEqual([['A', 1, 0.125]])
  })

  test('draws a region with nothing in it empty, and says so', () => {
    const [north] = frameOf(played, 1).shapes
    expect(north!.step).toBe(-1)
    expect(north!.formatted).toBe('Nothing in 2 Aug 2026')
    // Still the region: its outline is where it was.
    expect(north!.path).toBe(played.shapes[0]!.path)
  })

  test('shades a region from the shades every period shares', () => {
    const f = frameOf(played, 2)
    expect(f.shapes[0]!.step).toBe(4)
    expect(f.legend).toBe(played.frameLegend!)
  })

  test('takes a step of nothing as the lightest, as the wire leaves a zero out', () => {
    const m = structuredClone(played)
    m.shapes[0]!.frames![0] = { v: 0, f: '0' }
    expect(frameOf(m, 0).shapes[0]!.step).toBe(0)
  })

  test('leaves the map it came from as it was: every period together', () => {
    frameOf(played, 0)
    expect(played.markers.map((p) => p.value)).toEqual([8, 12])
    expect(played.shapes[0]!.step).toBe(5)
  })
})

describe("a period's bubbles", () => {
  // Sized against the largest in any period, as the server weighed them: the
  // key has to be drawn to the same rule, or it says a bubble means another
  // number than it does.
  test('are keyed against every period, not the one shown', () => {
    expect(periodValues(played).sort((a, b) => a - b)).toEqual([1, 2, 4, 5, 8])
  })

  test('are not keyed on a map that draws none', () => {
    expect(periodValues({ ...played, layers: ['scatter'] })).toEqual([])
  })
})
