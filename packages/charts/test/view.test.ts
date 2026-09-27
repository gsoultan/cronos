import { describe, expect, test } from 'bun:test'
import { viewport } from '../src/map/view'

/*
 * The window onto a map, as numbers. Every gesture is arithmetic on a viewBox,
 * so each can be checked without a browser: where the data lands, which point
 * stays under a zooming cursor, and where the limits stop a reader.
 */

// A city-sized box: the scale at which rounding to three decimals once put
// every dot on the same handful of points.
const city = { minX: 0.5122, minY: 0.3285, maxX: 0.5136, maxY: 0.3293 }

describe('fitting', () => {
  test('puts the data in the middle of the stage, all of it in view', () => {
    const port = viewport(city, 19)
    port.size(1000, 500)
    const v = port.view()
    expect(v.x).toBeLessThanOrEqual(city.minX)
    expect(v.x + v.w).toBeGreaterThanOrEqual(city.maxX)
    expect(v.y).toBeLessThanOrEqual(city.minY)
    expect(v.y + v.h).toBeGreaterThanOrEqual(city.maxY)
    expect(v.x + v.w / 2).toBeCloseTo((city.minX + city.maxX) / 2, 12)
    // The window has the stage's proportions, so nothing is stretched.
    expect(v.w / v.h).toBeCloseTo(2, 9)
    expect(port.moved()).toBe(false)
  })

  test('keeps a moved map where the reader left it when the stage resizes', () => {
    const port = viewport(city, 19)
    port.size(1000, 500)
    port.zoom(4)
    const before = port.view()
    const scale = port.scale()
    port.size(800, 500)
    const after = port.view()
    expect(port.scale()).toBeCloseTo(scale, 6)
    expect(after.x + after.w / 2).toBeCloseTo(before.x + before.w / 2, 12)
  })
})

describe('zooming', () => {
  test('keeps the point under the cursor under the cursor', () => {
    const port = viewport(city, 19)
    port.size(1000, 500)
    const at = (v: { x: number; y: number; w: number; h: number }) =>
      [v.x + 0.3 * v.w, v.y + 0.8 * v.h]
    const [x0, y0] = at(port.view())
    port.zoom(3, 300, 400)
    const [x1, y1] = at(port.view())
    expect(x1).toBeCloseTo(x0!, 12)
    expect(y1).toBeCloseTo(y0!, 12)
  })

  test('stops at the deepest zoom the tiles are served at', () => {
    const port = viewport(city, 12)
    port.size(1024, 512)
    for (let i = 0; i < 40; i++) port.zoom(2)
    // 1024 px across at zoom 12 is 1024 / (256 × 4096) of the world.
    expect(port.view().w).toBeCloseTo(1024 / (256 * 2 ** 12), 12)
    expect(port.can(1)).toBe(false)
    expect(port.can(-1)).toBe(true)
  })

  test('counts a 512-pixel tile as the size it is', () => {
    const port = viewport(city, 12, 512)
    port.size(1024, 512)
    for (let i = 0; i < 40; i++) port.zoom(2)
    expect(port.view().w).toBeCloseTo(1024 / (512 * 2 ** 12), 12)
  })

  test('stops at the whole world on the way out', () => {
    const port = viewport(city, 19)
    port.size(1000, 500)
    for (let i = 0; i < 40; i++) port.zoom(0.5)
    const v = port.view()
    expect(v.w).toBeCloseTo(1, 12)
    expect(v.h).toBeLessThanOrEqual(1)
    expect(v.x).toBeGreaterThanOrEqual(0)
    expect(port.can(-1)).toBe(false)
  })
})

describe('panning', () => {
  test('moves the window by the drag, in world units', () => {
    const port = viewport(city, 19)
    port.size(1000, 500)
    const before = port.view()
    port.pan(100, -50)
    const after = port.view()
    const perPx = before.w / 1000
    expect(after.x).toBeCloseTo(before.x - 100 * perPx, 12)
    expect(after.y).toBeCloseTo(before.y + 50 * perPx, 12)
    expect(port.moved()).toBe(true)
  })

  test('cannot drag the map off the edge of the world', () => {
    const port = viewport({ minX: 0.001, minY: 0.3, maxX: 0.01, maxY: 0.31 }, 19)
    port.size(1000, 500)
    port.pan(1e6, 0)
    expect(port.view().x).toBe(0)
  })

  test('fits again on request', () => {
    const port = viewport(city, 19)
    port.size(1000, 500)
    const fitted = port.view()
    port.zoom(8, 10, 10)
    port.pan(300, 200)
    port.fit()
    expect(port.view()).toEqual(fitted)
    expect(port.moved()).toBe(false)
  })
})
