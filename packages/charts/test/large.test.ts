import { describe, expect, test } from 'bun:test'
import { compact, grouped } from '../src/map/format'
import { describe as say, markersOf, ScreenIndex } from '../src/map/density'
import { cluster, placesIn } from '../src/map/cluster'
import { question, refiner } from '../src/map/detail'
import type { Cells, GeoMap, MapViewAsk } from '../src/types'

describe('numbers a viewer works out', () => {
  // The server's compact(), which formats every value it sends: a figure the
  // viewer adds up has to read like the ones beside it.
  test('read as the server writes them', () => {
    expect(compact(12_345)).toBe('12,345')
    expect(compact(1_234_567)).toBe('1.2M')
    expect(compact(2_000_000)).toBe('2M')
    expect(compact(2.5e9)).toBe('2.5B')
    expect(compact(-1500)).toBe('-1,500')
    expect(grouped(12.5)).toBe('12.50')
    expect(grouped(463)).toBe('463')
  })
})

const cells: Cells = {
  size: 0.001, x: [0.1, 0.2, 0.3], y: [0.1, 0.2, 0.3], n: [1, 40, 3], v: [7, 812, 3],
  l: ['Depot 7', '', ''],
}

describe('a cell of a large map', () => {
  test('of one place is that place', () => {
    expect(say(cells, 0)).toEqual({ label: 'Depot 7', sub: '7', place: 'Depot 7' })
  })
  test('of several says how many, and what its value is', () => {
    expect(say(cells, 1)).toEqual({ label: '40 locations', sub: '812' })
    expect(say({ ...cells, mean: true }, 1).sub).toBe('812 on average')
  })
})

// A cluster of cells counts the places in them, not the cells: three cells
// of forty-four places is forty-four locations.
test('a cluster of cells counts the places they hold', () => {
  const got = cluster(markersOf(cells), 1)
  expect(got).toHaveLength(1)
  expect(placesIn(got[0]!)).toBe(44)
  // Placed at the middle of its places, weighted by how many each cell
  // holds: (0.1·1 + 0.2·40 + 0.3·3) / 44, not the middle of three cells.
  expect(got[0]!.x).toBeCloseTo(9 / 44, 12)
})

test('the painted mark under the pointer is found by where it was painted', () => {
  const index = new ScreenIndex()
  index.add(100, 100, 4.5, 0)
  index.add(130, 100, 9, 1)
  expect(index.nearest(102, 101)).toBe(0)
  expect(index.nearest(137, 100)).toBe(1)
  expect(index.nearest(115, 100)).toBe(-1)
  // Across a bucket's edge.
  index.add(47, 47, 4.5, 2)
  expect(index.nearest(49, 49)).toBe(2)
})

describe('asking for a view', () => {
  const detail = { output: 'screen', block: 2, categories: ['Aurora', 'Baltic'] }

  test('is the same question for the same view', () => {
    const q = question(detail, { x: 0.1 + 1e-13, y: 0.2, w: 0.3, h: 0.4 }, 800, 600)
    expect(q).toEqual({
      output: 'screen', block: 2, view: [0.1, 0.2, 0.4, 0.6], width: 800, height: 600,
      categories: ['Aurora', 'Baltic'],
    })
  })

  // A reader zooms faster than a warehouse answers. An earlier view arriving
  // after a later one must not be painted over it.
  test('draws only the latest answer', async () => {
    const stage = fakeStage()
    const pending: { ask: MapViewAsk; resolve: (m: GeoMap) => void; signal: AbortSignal }[] = []
    const drawn: string[] = []
    const r = refiner(detail, (ask, signal) => new Promise((resolve) => pending.push({ ask, resolve, signal })),
      stage as unknown as HTMLElement, (m) => drawn.push(m.layers[0]!))

    r.follow({ x: 0, y: 0, w: 1, h: 1 })
    await sleep(200)
    r.follow({ x: 0.2, y: 0.2, w: 0.1, h: 0.1 })
    await sleep(200)
    expect(pending).toHaveLength(2)
    expect(pending[0]!.signal.aborted).toBe(true)

    pending[1]!.resolve(answer('later'))
    await sleep(0)
    pending[0]!.resolve(answer('earlier'))
    await sleep(0)
    expect(drawn).toEqual(['later'])
    expect(stage.busy).toBe(false)

    // The view it has just answered is not asked again.
    r.follow({ x: 0.2, y: 0.2, w: 0.1, h: 0.1 })
    await sleep(200)
    expect(pending).toHaveLength(2)
  })
})

function answer(tag: string): GeoMap {
  return { bounds: { minX: 0, minY: 0, maxX: 1, maxY: 1 }, layers: [tag], shapes: [], markers: [], arcs: [], legend: [] }
}

function fakeStage() {
  const s = {
    busy: false,
    clientWidth: 800,
    clientHeight: 600,
    isConnected: true,
    classList: {
      add: () => { s.busy = true },
      remove: () => { s.busy = false },
    },
  }
  return s
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))
