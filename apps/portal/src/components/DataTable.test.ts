import { describe, expect, test } from 'bun:test'
import { windowHeight } from './DataTable'

/*
 * A table is as tall as its rows.
 *
 * Its window was the height it was given, whatever the query returned — 420px
 * on a report — so customer-overview's three-row table sat on three hundred
 * pixels of nothing. It read as a load that had failed rather than as a short
 * answer, on the one table the report has.
 *
 * The cap is the other half, and the reason it exists: a report can return a
 * million rows, and the window is what the virtualiser mounts rows into.
 */
describe('a table window is sized to its rows', () => {
  test('three rows are three rows tall', () => {
    expect(windowHeight(3, 420)).toBe(120)
  })

  test('the cap applies once the rows reach it, and not before', () => {
    expect(windowHeight(10, 420)).toBe(400)
    expect(windowHeight(11, 420)).toBe(420)
  })

  test('a million rows are still one window, so they are still virtualised', () => {
    expect(windowHeight(1_000_000, 420)).toBe(420)
  })

  test('no rows keeps one row of height rather than collapsing to a line', () => {
    expect(windowHeight(0, 420)).toBe(40)
  })
})
