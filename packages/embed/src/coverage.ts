import type { Coverage, FilterDef } from './types'
import { el } from './dom'

/**
 * Renders "not affected by Period" on a block.
 *
 * The report format promises this, and it is the half everyone skips. A filter
 * that quietly applies to some blocks and not others is worse than one that
 * admits it — someone reading a filtered screen has no way to tell which
 * numbers moved, and will trust the ones that did not.
 *
 * Labels rather than names: the filter bar says "Period", so the block does
 * too. Falling back to the name is better than rendering nothing.
 */
export function unaffectedNote(cov: Coverage | undefined, filters: FilterDef[]): Node | null {
  const ignored = cov?.ignored ?? []
  if (ignored.length === 0) return null

  const labels = ignored.map((name) => filters.find((f) => f.name === name)?.label ?? name)
  const list =
    labels.length === 1
      ? labels[0]!
      : `${labels.slice(0, -1).join(', ')} and ${labels.at(-1)!}`

  return el('p', { class: 'unaffected', part: 'unaffected' },
    `Not affected by ${list}`)
}
