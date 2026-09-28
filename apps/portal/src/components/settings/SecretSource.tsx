import { SOURCE_LABEL } from '../../lib/secrets'
import type { SecretSource as Source } from '../../lib/types'

/*
 * Stored reads as done, missing reads as a problem, and the deployment's own
 * is neither — it works, and it is not this project's to change. Each is also
 * said in words, because a colour on its own says nothing to a screen reader
 * or to somebody who cannot tell red from green.
 */
const TONE: Record<Source, string> = {
  project: 'bg-good/15 text-delta-good',
  deployment: 'bg-sunken text-ink-secondary',
  missing: 'bg-critical/15 text-delta-bad',
}

/**
 * Where the value behind a name comes from.
 *
 * Undefined is a name the server did not list: nothing is stored under it and
 * no saved definition names it yet — a map in a report that has not been
 * saved. Whether the deployment answers it is unknown until one does, so this
 * says only what is certain.
 */
export function SecretSource({ source }: { source?: Source }) {
  return (
    <span data-testid="secret-source"
      className={`inline-flex shrink-0 items-center rounded-full px-2 py-px text-micro font-medium ${
        source ? TONE[source] : 'bg-sunken text-ink-secondary'}`}>
      {source ? SOURCE_LABEL[source] : 'Not stored here'}
    </span>
  )
}
