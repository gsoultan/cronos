import { useState } from 'react'
import { ApiError, testDataSource, type ProbeResult } from '../lib/api'

/**
 * Asks a source whether it is there, and says what it answered.
 *
 * The one question about a datasource that reading its definition cannot
 * answer, and the one whose wrong answer surfaces as a report failing at six
 * in the morning with a connection error nobody was watching for.
 *
 * A failure shows the driver's sentence rather than "could not connect": the
 * driver is the only party that knows whether this was a wrong password, a
 * closed port, or a database that does not exist, and those are three
 * different afternoons.
 */
export function TestConnection({ name }: { name: string }) {
  const [state, setState] = useState<'idle' | 'running'>('idle')
  const [result, setResult] = useState<ProbeResult | null>(null)

  async function go() {
    setState('running')
    setResult(null)
    try {
      setResult(await testDataSource(name))
    } catch (err) {
      setResult({
        source: name, ok: false, ms: 0,
        error: err instanceof ApiError ? err.message : 'Could not reach the server.',
      })
    } finally {
      setState('idle')
    }
  }

  if (state === 'running') {
    return <span className="text-small text-ink-muted">Testing…</span>
  }

  if (result) {
    return (
      <span className="flex items-baseline gap-2" data-testid="probe-result">
        <span className={`rounded-full px-2 py-px text-micro font-medium ${
          result.ok ? 'bg-good/15 text-delta-good' : 'bg-serious/20 text-ink'}`}>
          {result.ok ? `Answered in ${result.ms} ms` : 'No answer'}
        </span>
        {/* In full, and not truncated: the useful part of a driver error is
            usually the end of it. */}
        {result.error && (
          <span className="text-caption text-ink-secondary">{result.error}</span>
        )}
        <button type="button" onClick={() => setResult(null)}
          className="shrink-0 cursor-pointer text-small text-ink-muted underline">Dismiss</button>
      </span>
    )
  }

  return (
    <button type="button" onClick={go} data-testid="test-connection"
      className="cursor-pointer text-small text-ink-muted underline hover:text-ink">
      Test
    </button>
  )
}
