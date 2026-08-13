import { describe, expect, test } from 'bun:test'
import { size } from './StatTile'

/*
 * A hero figure stays inside its tile.
 *
 * The hero size is 48px and the tile is a quarter of the report width, so
 * "154,651.50" — the first number on the demo's first report — rendered
 * straight through the card beside it. Half the figure sat behind the
 * neighbouring tile, which reads as a broken page rather than as a number.
 *
 * The fixture's hero value is short, so nothing showed it until a real
 * warehouse produced a longer one.
 */
describe('a hero number is sized to fit', () => {
  test('a short figure gets the full hero size', () => {
    expect(size('4,120', true)).toBe('text-hero')
    expect(size('812', true)).toBe('text-hero')
  })

  test('the figure that overflowed does not', () => {
    expect(size('154,651.50', true)).not.toBe('text-hero')
  })

  test('and a very long one steps down again', () => {
    expect(size('1,254,651,000.75', true)).toBe('text-display')
  })

  test('a tile that is not the hero is unaffected', () => {
    // Every other tile was already at the smaller size and already fit.
    expect(size('154,651.50')).toBe('text-display')
    expect(size('4', false)).toBe('text-display')
  })
})
