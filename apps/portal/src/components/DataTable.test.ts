import { describe, expect, test } from 'bun:test'
import { renderCell, windowHeight } from './DataTable'
import type { Field } from '../lib/types'

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

/*
 * An engine-formatted cell the engine left empty stays empty. It writes a null
 * as a dash itself, so its empty string is a pivot's pair that no row reached,
 * and drawing a dash there made "nothing" and "null" the same answer.
 */
describe('an empty cell', () => {
  const field = (format?: string) =>
    ({ name: 'c1', label: 'Jul 2026', type: 'string', role: 'measure', format }) as Field

  test('the engine left empty stays empty', () => {
    expect(renderCell(field('preformatted'), '')).toBe('')
  })
  test('anywhere else is a dash', () => {
    expect(typeof renderCell(field(), '')).toBe('object')
  })
})
