import { connected } from '../lib/api'

/**
 * Says when the numbers on screen are made up.
 *
 * A reporting tool showing invented figures that look real is the worst thing
 * this product could do. Sample mode exists so the interface is workable
 * before a server does — and the moment it is not announced, somebody screens
 * hots a demo and a figure from it ends up in a board pack.
 *
 * Rendered in the shell rather than per page, because a reader who navigates
 * away from the page that told them should not be able to forget.
 */
export function SampleBanner() {
  if (connected()) return null

  return (
    <div data-testid="sample-banner"
      className="flex items-center justify-center gap-2 border-b border-line bg-accent-wash
                 px-4 py-1.5 text-caption text-ink-secondary">
      <span aria-hidden>●</span>
      <span>
        <b className="font-semibold text-ink">Sample data.</b>{' '}
        Not connected to a cronos server — every figure below is invented.
      </span>
    </div>
  )
}
