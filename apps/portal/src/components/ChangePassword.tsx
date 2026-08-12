import { useState } from 'react'
import { Button, PasswordInput } from '@mantine/core'
import { Field } from './form/Field'
import { ApiError, changePassword, connected } from '../lib/api'

/**
 * Changing your own password.
 *
 * Until this existed, nobody could rotate their own credential: a password
 * somebody had typed into the wrong window could only be changed by an
 * administrator with shell access, which in practice means it was not changed.
 *
 * The current one is required and checked by the server. A session lasts eight
 * hours and lives in a browser, and without that check anybody who borrowed
 * one for a minute could lock the owner out of their own account for good —
 * which is a worse outcome than the borrowed minute.
 */
export function ChangePassword() {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [again, setAgain] = useState('')
  const [state, setState] = useState<'idle' | 'busy' | 'done'>('idle')
  const [refused, setRefused] = useState('')

  const matches = next !== '' && next === again
  const longEnough = next.trim().length >= 12
  const ready = current !== '' && matches && longEnough

  async function submit() {
    setState('busy')
    setRefused('')
    try {
      await changePassword(current, next)
      setState('done')
      setCurrent(''); setNext(''); setAgain('')
    } catch (err) {
      setState('idle')
      setRefused(err instanceof ApiError ? err.message : 'Could not reach the server.')
    }
  }

  return (
    <div className="grid max-w-[520px] gap-4 p-4" data-testid="change-password">
      <Field label="Current password">
        <PasswordInput value={current} data-testid="current-password"
          disabled={!connected()}
          onChange={(e) => setCurrent(e.currentTarget.value)} />
      </Field>
      <Field label="New password" help="At least 12 characters. A passphrase is easier to remember and harder to guess.">
        <PasswordInput value={next} data-testid="new-password"
          disabled={!connected()}
          onChange={(e) => setNext(e.currentTarget.value)} />
      </Field>
      <Field label="New password again"
        error={again !== '' && !matches ? 'These do not match.' : undefined}>
        <PasswordInput value={again} data-testid="repeat-password"
          disabled={!connected()}
          onChange={(e) => setAgain(e.currentTarget.value)} />
      </Field>

      {!connected() && (
        <p className="text-small text-ink-secondary">
          This portal is showing sample data. Connect it to a cronos server to change
          a real password.
        </p>
      )}

      {refused && (
        <p role="alert" data-testid="password-error"
          className="rounded-md border border-serious/30 bg-serious/10 px-3 py-2 text-small text-ink">
          {refused}
        </p>
      )}

      {/* Said, and not with a toast. Somebody who has just changed a password
          wants to know it took, on the page they changed it on. */}
      {state === 'done' && (
        <p role="status" data-testid="password-changed"
          className="rounded-md border border-good/30 bg-good/10 px-3 py-2 text-small text-ink">
          Changed. Your other devices stay signed in until their sessions expire — a
          signed session has no record here to delete.
        </p>
      )}

      <div>
        <Button onClick={submit} loading={state === 'busy'} disabled={!ready || !connected()}
          data-testid="save-password">
          Change password
        </Button>
      </div>
    </div>
  )
}
