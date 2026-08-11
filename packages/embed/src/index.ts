import { CronosReport } from './element'

export { CronosReport }
export type {
  Block, ChartBlock, Coverage, Delta, FilterDef, FilterValues,
  ReportPayload, StatBlock, TableBlock,
} from './types'

/*
 * Registering is the side effect of importing, which is what a host page
 * expects from a single script tag. Guarded because a page that loads the
 * bundle twice — two widgets, two bundlers, one CDN and one npm — would
 * otherwise throw on the second, and the failure would land in our customer's
 * console with our name on it.
 */
export const TAG = 'cronos-report'

/**
 * Registers the element. Importing this package calls it; a host that controls
 * its own timing can call it again harmlessly.
 *
 * Returns silently on a server. Nothing to register there, and throwing would
 * turn "I imported a reporting widget" into a 500 on a page that never
 * rendered one.
 */
export function register(): void {
  if (typeof customElements === 'undefined') return
  if (!customElements.get(TAG)) customElements.define(TAG, CronosReport)
}

register()

declare global {
  interface HTMLElementTagNameMap {
    [TAG]: CronosReport
  }
}
