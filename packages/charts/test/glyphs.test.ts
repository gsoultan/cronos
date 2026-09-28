import { describe, expect, test } from 'bun:test'
import { glyphOf, paintGlyph } from '../src/map/glyphs'
import { sizeSteps } from '../src/map/keys'

/*
 * A category's glyph and a bubble's key. Both are claims a reader acts on:
 * that a square is a different category from a circle and no bigger, and that
 * a bubble the size of the largest ring is about that much.
 */

describe('a category is drawn in its own shape', () => {
  test('one per slot of the plot palette, folding past it as colour does', () => {
    expect([0, 1, 2, 5, undefined].map(glyphOf)).toEqual(['circle', 'square', 'triangle', 'triangle', 'circle'])
  })

  // Equal areas: a triangle drawn at the circle's radius would be a category
  // that looks like less, and a square one that looks like more.
  test('each at the area of the circle it stands for', () => {
    for (const glyph of ['square', 'triangle'] as const) {
      const pts: [number, number][] = []
      const ctx = {
        beginPath() {}, closePath() {},
        moveTo(x: number, y: number) { pts.push([x, y]) },
        lineTo(x: number, y: number) { pts.push([x, y]) },
        rect(x: number, y: number, w: number, h: number) { pts.push([x, y], [x + w, y], [x + w, y + h], [x, y + h]) },
        arc() {},
      } as unknown as CanvasRenderingContext2D
      paintGlyph(ctx, glyph, 50, 50, 10)
      let twice = 0
      for (let i = 0; i < pts.length; i++) {
        const [x1, y1] = pts[i]!
        const [x2, y2] = pts[(i + 1) % pts.length]!
        twice += x1 * y2 - x2 * y1
      }
      expect(Math.abs(twice) / 2).toBeCloseTo(Math.PI * 100, 9)
    }
  })
})

describe('a bubble key', () => {
  test('is three round numbers, the largest at or below the biggest value', () => {
    expect(sizeSteps([5200, 300, 1200])).toEqual({ refs: [5000, 1000, 200], hi: 5200 })
    expect(sizeSteps([0.37, 0.1])?.refs).toEqual([0.2, 0.05, 0.01])
  })

  // Bubbles below zero are sized from the lowest value, not from nothing, so
  // no ring could say what one means.
  test('says nothing it cannot say truthfully', () => {
    expect(sizeSteps([-5, 10])).toBeNull()
    expect(sizeSteps([0, 0])).toBeNull()
    expect(sizeSteps([])).toBeNull()
  })
})
