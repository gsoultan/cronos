import { useState } from 'react'
import { Button, TextInput } from '@mantine/core'
import { PageHeader } from '../components/PageHeader'
import { Field } from '../components/form/Field'
import { ChangePassword } from '../components/ChangePassword'
import { Tag } from '../components/StatusPill'
import { TwoFactorSetup } from '../forms/TwoFactorSetup'
import { relativeTime } from '../lib/format'
import { ApiError, endOtherSessions, endSession } from '../lib/api'
import type { Factor, FactorKind } from '../lib/security'

const CARD = 'mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'


export function AccountPage() {
  const [enrolling, setEnrolling] = useState(false)
  const [factors, setFactors] = useState<Factor[]>([])
  const [ending, setEnding] = useState(false)
  const [ended, setEnded] = useState('')

  if (enrolling) {
    return (
      <>
        <PageHeader title="Add a second factor"
          description="Four short steps. Nothing changes until you have proved it works." />
        <TwoFactorSetup onCancel={() => setEnrolling(false)}
          onDone={(kind: FactorKind, label) => {
            setFactors((f) => [...f, {
              id: `f${f.length + 1}`, kind, label, addedAt: new Date().toISOString(),
            }])
            setEnrolling(false)
          }} />
      </>
    )
  }

  const protectedNow = factors.length > 0

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

      <section className={CARD} data-testid="two-factor">
        <div className={HEAD}>
          <div>
            <h2 className="flex items-center gap-2 text-lead font-semibold text-ink">
              Two-factor authentication
              <span data-testid="two-factor-state"
                className={`rounded-full px-2 py-px text-micro font-medium ${
                  protectedNow ? 'bg-good/15 text-delta-good' : 'bg-serious/20 text-ink'}`}>
                {protectedNow ? 'On' : 'Off'}
              </span>
            </h2>
            <p className="mt-1 max-w-[68ch] text-small text-ink-secondary">
              A second factor means a stolen password is not enough on its own.
              We do not offer text-message codes — a phone number can be taken
              over, and offering them would suggest more protection than they give.
            </p>
          </div>
          <Button onClick={() => setEnrolling(true)}>
            {protectedNow ? 'Add another' : 'Turn on'}
          </Button>
        </div>

        {factors.length === 0 ? (
          <p className="px-4 py-8 text-center text-small text-ink-muted">
            No second factor yet. A passkey takes about fifteen seconds.
          </p>
        ) : (
          <ul>
            {factors.map((f) => (
              <li key={f.id}
                className="flex flex-wrap items-center gap-4 border-b border-line px-4 py-3 last:border-b-0">
                <span className="min-w-[200px] flex-1 font-medium text-ink">{f.label}</span>
                <Tag>{f.kind === 'passkey' ? 'passkey' : 'authenticator app'}</Tag>
                <span className="text-caption text-ink-muted">Added {relativeTime(f.addedAt)}</span>
                {/* Removing the last factor turns protection off, so it says so
                    rather than quietly downgrading the account. */}
                <Button variant="subtle" color="gray" size="xs"
                  onClick={() => setFactors((all) => all.filter((x) => x.id !== f.id))}>
                  {factors.length === 1 ? 'Remove — turns 2FA off' : 'Remove'}
                </Button>
              </li>
            ))}
          </ul>
        )}

        {protectedNow && (
          <div className="flex flex-wrap items-center gap-3 border-t border-line bg-sunken px-4 py-3">
            <p className="flex-1 text-small text-ink-secondary">
              <strong className="text-ink">10 recovery codes</strong> remain unused.
              Generating new ones invalidates the old set.
            </p>
            <Button variant="default" size="xs">Regenerate codes</Button>
          </div>
        )}
      </section>

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
