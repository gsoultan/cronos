import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError, connected, publishDefinition } from './api'

/**
 * Publishes a definition, and carries what the server said about it.
 *
 * The server validates deeply — it compiles every block before storing
 * anything — so a refusal is a sentence naming the field that is wrong. That
 * message is the whole value of publishing through the API rather than
 * validating twice, so it is surfaced rather than replaced with "could not
 * save".
 *
 * Unconnected, publishing is a no-op that reports success: sample mode has
 * nowhere to put a definition, and a form that refuses to submit would make
 * the interface untestable before a server exists.
 */
export function usePublish() {
  const queries = useQueryClient()
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function publish(yaml: string, expect?: string): Promise<boolean> {
    if (!connected()) return true

    setBusy(true)
    setError(null)
    try {
      await publishDefinition(yaml, expect)
      // The catalogue and every report are now stale. Invalidating rather than
      // refetching means a page nobody is looking at does not run a query.
      await queries.invalidateQueries()
      return true
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not reach the server.')
      return false
    } finally {
      setBusy(false)
    }
  }

  return { publish, error, busy, live: connected() }
}
