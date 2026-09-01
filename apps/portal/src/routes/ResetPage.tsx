import { useState } from 'react'
import { Button, PasswordInput } from '@mantine/core'
import { Brand } from '../components/Brand'
import { ApiError, resetPassword } from '../lib/api'

/**
 * Setting a new password from a link.
 *
 * Beside InvitationPage, and for the same reason: the person reading it cannot
 * sign in, which is the whole situation. Its only credential is the secret from
 * the link, which arrives in the fragment — a browser sends that to no server,
 * so it reaches no proxy log and no Referer header, and it is removed from the
 * address bar as soon as it is read.
 *
 * It deliberately does not name the account. An invitation says who it is for
 * because the recipient has no account yet and needs to know what they are
 * joining; a reset is for somebody who already knows, and echoing the address
 * back would turn a link anybody can hold into a way to confirm one.
 */
export function ResetPage() {
  const [secret] = useState(readSecret)
  const [password, setPassword] = useState('')
  const [again, setAgain] = useState('')
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const [done, setDone] = useState(false)

  const tooShort = password.length > 0 && password.length < 12
  const mismatch = again.length > 0 && again !== password
  const ready = password.length >= 12 && again === password && !busy && secret !== ''

  async function submit() {
    setBusy(true)
    setProblem(null)
    try {
      await resetPassword(secret, password)
      setDone(true)
    } catch (err) {
      // The server's own sentence. It is the only party that knows whether the
      // link was expired, spent or never issued — and it deliberately says the
      // same thing for all three.
      setProblem(err instanceof ApiError ? err.message : 'That did not work.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="grid min-h-screen place-items-center bg-canvas px-4">
      <div className="w-full max-w-[26rem]">
        <div className="mb-6 flex justify-center"><Brand /></div>

        <div className="rounded-lg border border-line bg-surface p-6 shadow-card">
          {done ? (
            <>
              <h1 className="text-lead font-semibold text-ink">Password changed</h1>
              <p className="mt-2 text-small text-ink-secondary">
                Every session this account had has ended, including any somebody
                else was using. Sign in with the new password — if you have a
                second factor, it will still ask for a code.
              </p>
              <Button component="a" href="/" fullWidth mt="lg" data-testid="reset-done">
                Sign in
              </Button>
            </>
          ) : (
            <>
              <h1 className="text-lead font-semibold text-ink">Choose a new password</h1>
              <p className="mt-2 mb-5 text-small text-ink-secondary">
                This link works once and expires an hour after it was sent.
              </p>

              {secret === '' && (
                <p role="alert" className="mb-4 text-small text-danger">
                  This link is incomplete. Ask for another from the sign-in page.
                </p>
              )}

              <form onSubmit={(e) => { e.preventDefault(); void submit() }} className="grid gap-4">
                <PasswordInput label="New password" value={password} autoFocus
                  data-testid="reset-password"
                  error={tooShort ? 'At least 12 characters.' : undefined}
                  onChange={(e) => setPassword(e.currentTarget.value)} />
                <PasswordInput label="Again" value={again}
                  data-testid="reset-again"
                  error={mismatch ? 'These do not match.' : undefined}
                  onChange={(e) => setAgain(e.currentTarget.value)} />

                {problem && (
                  <p role="alert" className="text-small text-danger">{problem}</p>
                )}

                <Button type="submit" fullWidth disabled={!ready} loading={busy}
                  data-testid="reset-submit">
                  Set password
                </Button>
              </form>
            </>
          )}
        </div>
      </div>
    </main>
  )
}

/**
 * The secret out of the fragment, and out of the address bar.
 *
 * Read once into state, so a re-render after the history is rewritten does not
 * find it gone. The same shape as the invitation page's, deliberately: two
 * pages that hold a secret from a link should not hold it two different ways.
 */
function readSecret(): string {
  const hash = globalThis.location?.hash ?? ''
  const found = new URLSearchParams(hash.replace(/^#/, '')).get('secret')
  if (found) {
    globalThis.history?.replaceState(null, '', globalThis.location.pathname)
  }
  return found ?? ''
}
