import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Button } from '@mantine/core'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError, currentUser, deleteSecret } from '../../lib/api'
import { relativeTime } from '../../lib/format'
import { keyValueProblem } from '../../lib/maps'
import {
  afterRemoval, afterSetting, brokenWithout, entryFor, envName, removalConsequence,
} from '../../lib/secrets'
import type { SecretEntry, SecretList, SecretUse } from '../../lib/types'
import { SecretSource } from './SecretSource'
import { SecretValueForm } from './SecretValueForm'

/**
 * One name: where its value comes from, what reads it, and what can be done.
 *
 * Set for a name nothing stores here, Replace for one that is — the same form
 * either way, and never filled in, because there is no value to fill it with.
 * Remove only for a stored one: the deployment's own is not this project's to
 * take away, and a missing one has nothing to remove.
 */
export function SecretRow({ entry, store, onSaid }: {
  entry: SecretEntry
  /** Whether the deployment can store anything. False hides every change. */
  store: boolean
  /** Says what the last change did, above the list. */
  onSaid: (sentence: string) => void
}) {
  const [mode, setMode] = useState<'idle' | 'set' | 'remove'>('idle')
  const [busy, setBusy] = useState(false)
  const [refused, setRefused] = useState('')
  const queries = useQueryClient()
  const stored = entry.source === 'project'

  /* The list that came back after the change decides the sentence. Asked of
     the cache rather than predicted: whether the deployment answers this name
     once the project's own value is gone is the server's to say. */
  const now = () => entryFor(queries.getQueryData<SecretList>(['secrets']), entry.name)

  async function remove() {
    setBusy(true)
    setRefused('')
    try {
      await deleteSecret(entry.name)
      await queries.invalidateQueries({ queryKey: ['secrets'] })
      onSaid(afterRemoval(entry.name, now()))
      setMode('idle')
    } catch (err) {
      setRefused(err instanceof ApiError ? err.message : 'Could not reach the server.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <li data-testid="secret-row" data-secret={entry.name}
      className="border-b border-line px-4 py-3 last:border-b-0">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <code className="min-w-44 font-mono text-small font-semibold break-all text-ink">
          {entry.name}
        </code>
        <SecretSource source={entry.source} />
        <UsedBy uses={entry.usedBy} />
        {stored && entry.updatedAt && (
          <span className="text-caption text-ink-muted" data-testid="secret-set-at">
            set {relativeTime(entry.updatedAt)}
            {entry.updatedBy && entry.updatedBy === currentUser()?.id ? ' by you' : ''}
          </span>
        )}
        {store && mode === 'idle' && (
          <span className="ml-auto flex shrink-0 gap-2">
            {/* Named in full for a screen reader, where a list of rows is a
                list of buttons all called Set. */}
            <Button size="xs" variant={entry.source === 'missing' ? 'filled' : 'default'}
              data-testid={stored ? 'secret-replace' : 'secret-set'}
              aria-label={`${stored ? 'Replace' : 'Set'} ${entry.name}`}
              onClick={() => { setMode('set'); onSaid('') }}>
              {stored ? 'Replace' : 'Set'}
            </Button>
            {stored && (
              <Button size="xs" variant="subtle" color="gray" data-testid="secret-remove"
                aria-label={`Remove ${entry.name}`}
                onClick={() => { setMode('remove'); onSaid('') }}>
                Remove
              </Button>
            )}
          </span>
        )}
      </div>

      {entry.source === 'missing' && entry.usedBy.length > 0 && (
        <p className="mt-1 text-caption text-delta-bad" data-testid="secret-broken">
          Nothing answers this name: {brokenWithout(entry.usedBy)}.
          {!store && <> Set <code className="font-mono">{envName(entry.name)}</code> on the server to fix it.</>}
        </p>
      )}

      {mode === 'set' && (
        <div className="mt-3 max-w-lg">
          <SecretValueForm name={entry.name}
            label={stored ? `New value for ${entry.name}` : `Value for ${entry.name}`}
            help={setHelp(entry)}
            submitLabel={stored ? 'Replace' : 'Save'}
            check={(v) => keyValueProblem(entry.name, v)}
            onCancel={() => setMode('idle')}
            onSaved={() => { setMode('idle'); onSaid(afterSetting(entry.name, now())) }} />
        </div>
      )}

      {mode === 'remove' && (
        <div role="group" aria-label={`Remove ${entry.name}`} data-testid="secret-remove-ask"
          className="mt-3 grid max-w-prose gap-2 rounded-md border border-line bg-sunken p-3 text-small">
          <p className="text-ink">
            <strong>Remove {entry.name}?</strong> {removalConsequence(entry)}
          </p>
          <span className="flex gap-2">
            <Button size="xs" color="red" loading={busy} data-testid="secret-remove-confirm"
              onClick={() => void remove()}>
              Remove
            </Button>
            <Button size="xs" variant="default" disabled={busy} onClick={() => setMode('idle')}>
              Keep it
            </Button>
          </span>
        </div>
      )}

      {refused && (
        <p role="alert" data-testid="secret-refused" className="mt-2 text-small text-ink">{refused}</p>
      )}
    </li>
  )
}

/**
 * What reads this name, each linked to where it can be looked at.
 *
 * A report opens on its own page, where a missing key shows as the note on
 * its map. A source has no page but its editor, which is also where its
 * password is changed — so that is where its link goes.
 */
function UsedBy({ uses }: { uses: SecretUse[] }) {
  if (uses.length === 0) {
    return <span className="min-w-40 flex-1 text-small text-ink-muted">Nothing uses it</span>
  }
  return (
    <span data-testid="secret-used-by"
      className="flex min-w-40 flex-1 flex-wrap items-baseline gap-x-2 text-small text-ink-secondary">
      <span className="text-ink-muted">Used by</span>
      {uses.map((u) => (
        <span key={`${u.kind}/${u.name}`} className="whitespace-nowrap">
          <span className="text-ink-muted">{u.kind === 'Report' ? 'report' : 'source'} </span>
          {u.kind === 'Report' ? (
            <Link to="/reports/$name" params={{ name: u.name }} className="text-accent underline">
              {u.name}
            </Link>
          ) : (
            <Link to="/data/sources/$name/edit" params={{ name: u.name }} className="text-accent underline">
              {u.name}
            </Link>
          )}
        </span>
      ))}
    </span>
  )
}

/**
 * Said before the value is typed: what saving it will change.
 *
 * The value is stored as typed, and that is right for a password too: a
 * connection string that reads one inside a URL names it `${secret:name|url}`,
 * and the server encodes it there.
 */
function setHelp(entry: SecretEntry): string {
  const said = (() => {
    switch (entry.source) {
      case 'project':
        return 'Replaces the stored value. The old one is not shown here, and cannot be.'
      case 'deployment':
        return 'The server’s environment answers this today. A value stored here takes its '
          + 'place, for this project only.'
      default:
        return entry.usedBy.length > 0
          ? 'What reads it starts working as soon as this is saved — nothing needs restarting.'
          : 'Stored sealed. It is never shown again, to anybody.'
    }
  })()
  return said
}
