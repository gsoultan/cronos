import { describe, expect, test } from 'bun:test'
import { arc, bar, fit } from '../src/frame'
import { thin } from '../src/axes'
import { stops } from '../src/blocks/heatmap'

/*
 * The geometry every chart is drawn with, where a mistake is a chart that
 * renders blank with nothing in the console: a label cut to its room, a bar
 * rounded at its value's end, a slice of a ring, and the labels an axis
 * leaves out so the rest can be read.
 */

// Seven pixels a character, like a narrow sans at 12px.
const m = (text: string) => text.length * 7

describe('a label', () => {
  test('that fits is left alone', () => {
    expect(fit('Scotland', 100, 12, m)).toBe('Scotland')
  })
  test('that does not is cut, with an ellipsis, to the room it has', () => {
    const cut = fit('Aurora Freight International', 100, 12, m)
    expect(cut.endsWith('…')).toBe(true)
    expect(m(cut)).toBeLessThanOrEqual(100)
  })
  test('with no room at all is nothing, not an ellipsis alone', () => {
    expect(fit('Scotland', 4, 12, m)).toBe('')
  })
})

describe('a bar', () => {
  test('is rounded at its value end and square on the axis', () => {
    const up = bar(10, 20, 30, 100, 'up', 4)
    // Starts on the axis, square, and curves only at the top.
    expect(up.startsWith('M10 120V24Q10 20 14 20')).toBe(true)
    const down = bar(10, 20, 30, 100, 'down', 4)
    expect(down.startsWith('M10 20V116Q10 120 14 120')).toBe(true)
  })
  test('thinner than its rounding takes half its width as the radius', () => {
    expect(bar(0, 0, 4, 50, 'up', 4)).toContain('Q0 0 2 0')
  })
  test('of no size is no path at all', () => {
    expect(bar(0, 0, 0, 10, 'right')).toBe('')
    expect(bar(0, 0, 10, 0, 'right')).toBe('')
  })
})

describe('a slice', () => {
  test('of a pie closes on the centre', () => {
    expect(arc(100, 100, 0, 50, 0, Math.PI / 2)).toMatch(/L100 100Z$/)
  })
  test('of a donut is two arcs, one each way', () => {
    const d = arc(100, 100, 30, 50, 0, Math.PI / 2)
    expect(d.match(/A/g)?.length).toBe(2)
    expect(d).toContain('0 0 1')
    expect(d).toContain('0 0 0')
  })
  test('the whole way round is drawn in halves, where one arc would draw nothing', () => {
    expect(arc(100, 100, 0, 50, 0, Math.PI * 2).match(/A/g)?.length).toBe(2)
  })
  test('past half a turn takes the large arc', () => {
    expect(arc(100, 100, 0, 50, 0, Math.PI * 1.5)).toContain('0 1 1')
  })
})

describe('an axis of categories', () => {
  test('labels every band while the widest name fits one', () => {
    expect(thin(40, 80)).toBe(1)
  })
  test('labels every second or third where they would touch', () => {
    expect(thin(60, 40)).toBe(2)
    expect(thin(60, 24)).toBe(3)
  })
  test('cuts a very long name rather than thinning to nothing', () => {
    expect(thin(900, 80)).toBe(thin(140, 80))
  })
})

describe("a heatmap's key", () => {
  const cell = (value: number, step: number, empty = false) =>
    ({ row: 'r', column: 'c', value, formatted: String(value), step, empty })
  test('spans the lowest to the highest value drawn in each shade', () => {
    expect(stops([cell(10, 0), cell(30, 0), cell(20, 0), cell(90, 5)])).toEqual([
      { step: 0, from: '10', to: '30' },
      { step: 5, from: '90', to: '90' },
    ])
  })
  test('leaves out a shade nothing was drawn in, and the cells no row matched', () => {
    expect(stops([cell(0, 0, true), cell(50, 3)]).map((s) => s.step)).toEqual([3])
  })
})
