import { useState } from 'react'
import { Button } from '@mantine/core'
import { ApiError } from '../../lib/api'
import { useSecrets } from '../../lib/useSecrets'
import { AddSecret } from './AddSecret'
import { SecretRow } from './SecretRow'

const CARD = 'mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'

/**
 * A project's secrets, and what each one is for.
 *
 * Every name a definition uses is listed whether or not anything answers it,
 * and that is the reason this tab exists as much as storing is: a map drawn
 * with no basemap and a warehouse that will not open are both a secret nobody
 * set, and until now the only place that said so was the server's log.
 *
 * Nothing on this tab can show a value, because nothing on the server returns
 * one. Replacing a secret is typing a new one; knowing what the old one was is
 * a question for wherever it was copied from.
 */
export function SecretsPanel() {
  const { data, error, isPending, live } = useSecrets()
  const [adding, setAdding] = useState(false)
  const [said, setSaid] = useState('')

  if (!live) {
    return (
      <section className={CARD}>
        <p className="p-4 text-small text-ink-secondary">
          Secrets are kept by the server, sealed with its own key, so this tab needs a
          connected one. The sample data names none.
        </p>
      </section>
    )
  }
  if (isPending) {
    return <p data-testid="secrets-loading" className="p-8 text-center text-ink-muted">Loading…</p>
  }
  if (error || !data) return <Unreadable error={error} />

  return (
    <section className={CARD} data-testid="secrets-panel">
      <div className={HEAD}>
        <div>
          <h2 className="text-lead font-semibold text-ink">Secrets</h2>
          <p className="mt-1 max-w-prose text-small text-ink-secondary">
            Passwords, tokens and keys this project’s definitions name as{' '}
            <code className="font-mono text-caption">{'${secret:name}'}</code>. A value goes in
            and never comes back out — not to you, not to anybody — and takes effect without a
            restart.
          </p>
        </div>
        {data.store && (
          <Button onClick={() => { setAdding((v) => !v); setSaid('') }} data-testid="add-secret">
            {adding ? 'Cancel' : 'Add a secret'}
          </Button>
        )}
      </div>

      {!data.store && <NoKey />}

      {adding && (
        <AddSecret list={data} onAdded={(sentence) => { setAdding(false); setSaid(sentence) }} />
      )}

      {said && (
        <p role="status" data-testid="secrets-said"
          className="border-b border-line bg-good/10 px-4 py-2 text-small text-ink">
          {said}
        </p>
      )}

      {data.secrets.length === 0 ? (
        <p className="px-4 py-8 text-center text-small text-ink-muted" data-testid="no-secrets">
          Nothing is stored here and no definition names a secret. A datasource password and a
          map’s key both land here when somebody sets one.
        </p>
      ) : (
        <ul>
          {data.secrets.map((entry) => (
            <SecretRow key={entry.name} entry={entry} store={data.store} onSaid={setSaid} />
          ))}
        </ul>
      )}
    </section>
  )
}

/**
 * The deployment cannot seal anything, and says what to do about it.
 *
 * Above the list rather than instead of it: which names are in use and which of
 * them nothing answers is as true on a server with no key as on one with, and
 * it is the half of this tab somebody fixing a broken map most needs.
 */
function NoKey() {
  return (
    <div role="note" data-testid="secrets-no-key"
      className="border-b border-line bg-serious/10 px-4 py-3 text-small text-ink">
      <p>
        <strong>This deployment has no CRONOS_SECRETS_KEY, so secrets cannot be stored
        here.</strong> Set one and restart the server — see “Secrets” in
        docs/deploying.md.
      </p>
      <p className="mt-1 text-ink-secondary">
        Until then every value comes from the server’s environment, as{' '}
        <code className="font-mono text-caption">CRONOS_SECRET_&lt;NAME&gt;</code>. The list
        below still says which names are answered and which are not.
      </p>
    </div>
  )
}

/**
 * The list could not be read, in words that match why.
 *
 * A refusal is the ordinary answer for somebody whose role changed under them.
 * No endpoint at all is a server with no definition store — the endpoint is
 * mounted wherever there is one — or one older than stored secrets, and either
 * way every value comes from its environment, which is worth saying rather
 * than showing an error.
 */
function Unreadable({ error }: { error: unknown }) {
  const status = error instanceof ApiError ? error.status : 0
  const text = status === 404
    ? 'This server has nowhere to store a secret: it runs without a definition store, or '
      + 'predates stored secrets. Every value comes from its environment, as CRONOS_SECRET_<NAME>.'
    : error instanceof ApiError ? error.message : 'Could not reach the server.'
  return (
    <section className={CARD} data-testid="secrets-unreadable">
      <p className="p-4 text-small text-ink-secondary">{text}</p>
    </section>
  )
}
