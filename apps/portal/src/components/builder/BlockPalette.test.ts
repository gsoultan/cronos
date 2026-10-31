import { describe, expect, test } from 'bun:test'
import { GROUPS, PALETTE } from './BlockPalette'

/*
 * The groups are a second list of the same kinds, and a kind added to PALETTE
 * and not to a group would be a block nobody can add from the editor.
 */
describe('the palette groups', () => {
  test('offer every kind exactly once', () => {
    const grouped = GROUPS.flatMap((g) => g.kinds)
    expect(grouped.toSorted()).toEqual(PALETTE.map((p) => p.kind).toSorted())
  })
})
