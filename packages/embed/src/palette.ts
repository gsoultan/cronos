/**
 * How many categorical colours the viewer has.
 *
 * The palette lives in styles.ts as CSS custom properties, because custom
 * properties are the one thing that crosses a shadow boundary and so are this
 * package's whole theming API. This constant is the count, which script needs
 * and CSS cannot tell it.
 *
 * A series past the last slot folds into the last slot rather than generating
 * a hue. Generated hues are the ones that collide under colour-vision
 * deficiency, and a cycled palette is worse still: it gives two different
 * series the same colour with nothing on the chart saying they differ.
 */
export const PALETTE_SIZE = 8

/**
 * How many are safe when every series is beside every other.
 *
 * A stacked bar fixes its series in one order, so only neighbouring pairs have
 * to be told apart and all eight hold. A scatter puts them in the same space,
 * where every pair is adjacent — and only the first three slots clear the
 * contrast floors against every other slot. The server caps it; this is the
 * number it caps to, kept here so the two cannot drift apart silently.
 */
export const PLOT_PALETTE_SIZE = 3

/** The slot to actually paint with, folding the overflow into the last. */
export function slotOf(slot: number, size = PALETTE_SIZE): number {
  return Math.min(Math.max(slot, 0), size - 1) + 1
}

/**
 * How many shades a map's sequential ramp has.
 *
 * Six bands rather than a continuous gradient, matching the server's
 * RampSteps: a reader answers "same band or not" far better than "darker or
 * not", and a legend of six is still readable at the size a report gives it.
 */
export const RAMP_STEPS = 6

/**
 * How many steps the ordinal ramp has.
 *
 * Five, not six. The ordinal ramp is discrete marks a reader compares to each
 * other rather than a continuum read against a legend, so each step has to be
 * visibly different from its neighbour — and the range between "light enough
 * to still clear the surface" and "as dark as the hue goes" only holds five of
 * those gaps. A sixth stage folds into the last.
 */
export const ORDINAL_STEPS = 5

/** The ordinal step to paint with, folding the overflow into the last. */
export function stepOf(rank: number): number {
  return Math.min(Math.max(rank, 0), ORDINAL_STEPS - 1) + 1
}
