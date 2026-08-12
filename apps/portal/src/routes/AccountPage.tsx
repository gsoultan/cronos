import { useEffect, useState } from 'react'
import { Button, TextInput } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { Field } from '../components/form/Field'
import { ChangePassword } from '../components/ChangePassword'
import { TwoFactorSetup } from '../forms/TwoFactorSetup'
import { relativeTime } from '../lib/format'
import {
  ApiError, endOtherSessions, endSession, factor, newRecoveryCodes, removeFactor,
} from '../lib/api'

const CARD = 'mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'


export function AccountPage() {
  // Bumped after anything that changes what the server would say, which
  // remounts the panel below rather than threading a refetch through it.
  const [reloads, setReloads] = useState(0)
  const [ending, setEnding] = useState(false)
  const [ended, setEnded] = useState('')

  return (
    <>
      <PageHeader title="Your account" description="Profile, sign-in and devices." />

      <section className={CARD}>
        <div className={HEAD}>
          <h2 className="text-lead font-semibold text-ink">Profile</h2>
        </div>
        <div className="grid max-w-[520px] gap-4 p-4">
          <Field label="Name">
            <TextInput defaultValue="Dewi Rahayu" />
          </Field>
          <Field label="Email"
            help="Used to sign in and to send you anything you have subscribed to.">
            <TextInput defaultValue="dewi@acme.com" type="email" />
          </Field>
          <div><Button>Save changes</Button></div>
        </div>
      </section>

      <section className={CARD} data-testid="password">
        <div className={HEAD}>
          <div>
            <h2 className="text-lead font-semibold text-ink">Password</h2>
            <p className="mt-1 text-small text-ink-secondary">
              Yours to change. Nobody else can change it for you, and an administrator
              cannot read it.
            </p>
          </div>
        </div>
        <ChangePassword />
      </section>

      <TwoFactor onChanged={() => setReloads((n) => n + 1)} key={reloads} />

      <section className={CARD}>
        <div className={HEAD}>
          <div>
            <h2 className="text-lead font-semibold text-ink">Sessions</h2>
            <p className="mt-1 text-small text-ink-secondary">
              Signing in gives your browser a token that lasts eight hours. There
              is no list of devices here because there is none to show — nothing
              is recorded when a session starts, so one cannot be told from
              another.
            </p>
          </div>
          <Button variant="default" color="red" loading={ending}
            data-testid="end-sessions"
            onClick={() => {
              setEnding(true)
              setEnded('')
              endOtherSessions()
                .then(() => setEnded('Done. Every other session has ended.'))
                .catch((err: unknown) =>
                  setEnded(err instanceof ApiError ? err.message : 'Could not end them.'))
                .finally(() => setEnding(false))
            }}>
            Sign out everywhere else
          </Button>
        </div>
        <div className="px-4 py-3">
          <p className="text-small text-ink-secondary">
            What this does instead is end every session at once and give this
            browser a new one — so you stay signed in here and every other
            machine is signed out. Press it if a laptop or phone has gone
            missing. It does not change your password.
          </p>
          {ended && (
            <p role="status" data-testid="sessions-ended"
              className="mt-3 text-small text-ink">{ended}</p>
          )}
        </div>
      </section>

      <section className={CARD}>
        <div className="flex flex-wrap items-center justify-between gap-4 p-4">
          <div>
            <h2 className="text-lead font-semibold text-ink">Sign out</h2>
            <p className="mt-1 text-small text-ink-secondary">
              Ends this session here. If you signed in through your
              organisation's directory, it ends that session too — so the next
              person at this machine is asked who they are.
            </p>
          </div>
          <Button variant="default" color="red" data-testid="account-sign-out"
            onClick={() => void endSession()}>
            Sign out
          </Button>
        </div>
      </section>
    </>
  )
}

/**
 * What actually protects this account.
 *
 * Read from the server on every mount. The list it replaces was React state
 * seeded with an empty array — so "Two-factor authentication: Off" was true
 * only because nothing had been clicked in this tab, and it said "On" for the
 * rest of the session after a wizard that verified nothing.
 */
function TwoFactor({ onChanged }: { onChanged: () => void }) {
  const [state, setState] = useState<Awaited<ReturnType<typeof factor>> | null>(null)
  const [enrolling, setEnrolling] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [code, setCode] = useState('')
  const [problem, setProblem] = useState('')
  const [fresh, setFresh] = useState<string[]>([])

  useEffect(() => {
    let live = true
    factor()
      .then((out) => { if (live) setState(out) })
      .catch(() => { if (live) setState({ enrolled: false }) })
    return () => { live = false }
  }, [])

  if (enrolling) {
    return (
      <TwoFactorSetup onCancel={() => setEnrolling(false)}
        onDone={() => { setEnrolling(false); onChanged() }} />
    )
  }

  const on = state?.enrolled ?? false

  return (
    <section className={CARD} data-testid="two-factor">
      <div className={HEAD}>
        <div>
          <h2 className="flex items-center gap-2 text-lead font-semibold text-ink">
            Two-factor authentication
            <span data-testid="two-factor-state"
              className={`rounded-full px-2 py-px text-micro font-medium ${
                on ? 'bg-good/15 text-delta-good' : 'bg-serious/20 text-ink'}`}>
              {state === null ? '…' : on ? 'On' : 'Off'}
            </span>
          </h2>
          <p className="mt-1 max-w-[68ch] text-small text-ink-secondary">
            A second factor means a stolen password is not enough on its own.
            We do not offer text-message codes — a phone number can be taken
            over, and offering them would suggest more protection than they give.
          </p>
        </div>
        {!on && state !== null && (
          <Button onClick={() => setEnrolling(true)} data-testid="turn-on-2fa">Turn on</Button>
        )}
      </div>

      {on && (
        <>
          <div className="flex flex-wrap items-center gap-4 border-b border-line px-4 py-3">
            <span className="min-w-[200px] flex-1 font-medium text-ink">{state?.label}</span>
            <span className="text-caption text-ink-muted">
              Added {state?.addedAt ? relativeTime(state.addedAt) : ''}
            </span>
            <Button variant="subtle" color="gray" size="xs"
              onClick={() => { setRemoving((v) => !v); setProblem('') }}>
              {removing ? 'Keep it on' : 'Turn off'}
            </Button>
          </div>

          <div className="flex flex-wrap items-center gap-3 border-b border-line bg-sunken px-4 py-3">
            <p className="flex-1 text-small text-ink-secondary">
              <strong className="text-ink">{state?.remainingCodes ?? 0} recovery codes</strong>{' '}
              remain unused. Generating new ones retires the old set.
            </p>
            <Button variant="default" size="xs"
              onClick={() => {
                setProblem('')
                newRecoveryCodes()
                  .then((out) => { setFresh(out.recoveryCodes); onChanged() })
                  .catch((err: unknown) =>
                    setProblem(err instanceof ApiError ? err.message : 'Could not generate them.'))
              }}>
              Regenerate codes
            </Button>
          </div>

          {fresh.length > 0 && (
            <div className="border-b border-line px-4 py-3">
              <p className="mb-2 text-small text-ink">
                <strong>Save these now.</strong> This is the only time they are shown.
              </p>
              <ul data-testid="fresh-codes"
                className="grid grid-cols-2 gap-2 rounded-lg bg-sunken p-4 font-mono text-small">
                {fresh.map((c) => <li key={c} className="text-ink">{c}</li>)}
              </ul>
            </div>
          )}

          {removing && (
            <div className="border-b border-line px-4 py-3">
              {/* A current code, because otherwise a stolen session strips the
                  factor off the account it stole — at the moment the factor is
                  the only thing left. */}
              <Field label="Enter a current code to turn it off"
                help="Or one of your recovery codes, if you no longer have the app.">
                <div className="flex flex-wrap items-center gap-2">
                  <TextInput value={code} inputMode="numeric" w={200} placeholder="123456"
                    data-testid="remove-2fa-code"
                    onChange={(e) => { setCode(e.currentTarget.value); setProblem('') }} />
                  <Button color="red" variant="default" data-testid="remove-2fa"
                    onClick={() => {
                      removeFactor(code.trim())
                        .then(() => { setRemoving(false); onChanged() })
                        .catch((err: unknown) =>
                          setProblem(err instanceof ApiError ? err.message : 'Could not turn it off.'))
                    }}>
                    Turn off two-factor
                  </Button>
                </div>
              </Field>
            </div>
          )}
        </>
      )}

      {problem && (
        <p role="alert" data-testid="two-factor-error"
          className="border-b border-line bg-serious/10 px-4 py-2 text-small text-ink">
          {problem}
        </p>
      )}
    </section>
  )
}
