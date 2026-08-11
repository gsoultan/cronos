import { CronosReport } from './element'

export { CronosReport }
export type {
  Block, BarBlock, Coverage, Delta, FilterDef, FilterValues,
  ReportPayload, StatBlock, TableBlock,
} from './types'

/*
 * Registering is the side effect of importing, which is what a host page
 * expects from a single script tag. Guarded because a page that loads the
 * bundle twice — two widgets, two bundlers, one CDN and one npm — would
 * otherwise throw on the second, and the failure would land in our customer's
 * console with our name on it.
 */
const TAG = 'cronos-report'
if (!customElements.get(TAG)) customElements.define(TAG, CronosReport)

declare global {
  interface HTMLElementTagNameMap {
    [TAG]: CronosReport
  }
}
