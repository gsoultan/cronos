import { useEffect, useState } from 'react'
import { Button, TextInput } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { EmptyState } from '../components/EmptyState'
import { Field } from '../components/form/Field'
import { ChangePassword } from '../components/ChangePassword'
import { TwoFactorSetup } from '../forms/TwoFactorSetup'
import { relativeTime } from '../lib/format'
import {
  ApiError, connected, endOtherSessions, endSession, factor, newRecoveryCodes,
  profile, removeFactor, rename,
} from '../lib/api'

const CARD = 'mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'


export function AccountPage() {
  // Bumped after anything that changes what the server would say, which
  // remounts the panel below rather than threading a refetch through it.
  const [reloads, setReloads] = useState(0)
  const [enrolling, setEnrolling] = useState(false)
  const [ending, setEnding] = useState(false)
  const [ended, setEnded] = useState('')

  /* Every panel below one is the server's answer about a real account. On
     samples there is no account and no server, so they would each show the same
     "not connected" error in a different shape — which reads as four things
     broken rather than one thing absent. */
  if (!connected()) {
    return (
      <>
        <PageHeader title="Your account" description="Profile, sign-in and security." />
        <EmptyState title="This is the sample portal"
          description="Your password, your second factor and your sessions all belong to an account on a cronos server, and this build is not connected to one. Point VITE_CRONOS_API at a server and sign in to manage them." />
      </>
    )
  }

  if (enrolling) {
    /* The whole page, not a card between the password and the sessions: this is
       a four-step task with its own Back and Continue, and leaving the rest of
       the account around it puts two sets of controls on screen at once. */
    return (
      <>
        <PageHeader title="Add a second factor"
          description="Three short steps. Nothing changes until you have proved it works." />
        <TwoFactorSetup onCancel={() => setEnrolling(false)}
          onDone={() => { setEnrolling(false); setReloads((n) => n + 1) }} />
      </>
    )
  }

  return (
    <>
      <PageHeader title="Your account" description="Profile, sign-in and security." />

      <ProfileCard key={`profile-${reloads}`} onSaved={() => setReloads((n) => n + 1)} />

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

      <TwoFactor key={`factor-${reloads}`} enrolling={enrolling} onEnrol={setEnrolling}
        onChanged={() => setReloads((n) => n + 1)} />

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
function TwoFactor({ enrolling, onEnrol, onChanged }: {
  enrolling: boolean
  onEnrol: (v: boolean) => void
  onChanged: () => void
}) {
  const [state, setState] = useState<Awaited<ReturnType<typeof factor>> | null>(null)
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

  if (enrolling) return null

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
          <Button onClick={() => onEnrol(true)} data-testid="turn-on-2fa">Turn on</Button>
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

/**
 * Who this session belongs to.
 *
 * Read from the server rather than typed into the source, which is what it was:
 * a connected deployment showed "Dewi Rahayu / dewi@acme.com" whoever was signed
 * in, above a Save button that did nothing. Being wrong about whose account this
 * is, on the page that changes its password and its second factor, is the worst
 * place in the product to be wrong.
 */
function ProfileCard({ onSaved }: { onSaved: () => void }) {
  const [me, setMe] = useState<Awaited<ReturnType<typeof profile>> | null>(null)
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [said, setSaid] = useState('')

  useEffect(() => {
    let live = true
    profile()
      .then((out) => { if (live) { setMe(out); setName(out.name ?? '') } })
      .catch(() => { if (live) setMe(null) })
    return () => { live = false }
  }, [])

  const changed = me !== null && name.trim() !== (me.name ?? '') && name.trim() !== ''

  return (
    <section className={CARD} data-testid="profile">
      <div className={HEAD}>
        <h2 className="text-lead font-semibold text-ink">Profile</h2>
      </div>
      <div className="grid max-w-[520px] gap-4 p-4">
        <Field label="Name">
          <TextInput value={name} data-testid="profile-name"
            disabled={me === null || !me.account}
            onChange={(e) => { setName(e.currentTarget.value); setSaid('') }} />
        </Field>
        <Field label="Email"
          help="What you sign in with. Changing it needs the new address proved before the old one stops working, which is not built — ask an administrator.">
          {/* Read-only rather than an input that discards what is typed. */}
          <TextInput value={me?.email ?? ''} type="email" readOnly disabled
            data-testid="profile-email" />
        </Field>

        {me && !me.account && (
          <p className="text-small text-ink-secondary">
            This is a machine credential rather than a person, so there is no
            profile to change.
          </p>
        )}

        <div className="flex items-center gap-3">
          <Button disabled={!changed || busy} loading={busy} data-testid="save-profile"
            onClick={() => {
              setBusy(true)
              rename(name.trim())
                .then(() => { setSaid('Saved.'); onSaved() })
                .catch((err: unknown) =>
                  setSaid(err instanceof ApiError ? err.message : 'Could not save that.'))
                .finally(() => setBusy(false))
            }}>
            Save changes
          </Button>
          {said && <span role="status" className="text-small text-ink-secondary">{said}</span>}
        </div>
      </div>
    </section>
  )
}
