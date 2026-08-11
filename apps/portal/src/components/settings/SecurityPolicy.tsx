import { useState } from 'react'
import { Button, Select, Switch } from '@mantine/core'
import { Field } from '../form/Field'
import { readiness, type SecurityPolicy as Policy } from '../../lib/security'
import type { Person } from '../../lib/people'

interface Props {
  orgName: string
  members: Person[]
  twoFactorOn: (p: Person) => boolean
  canAdmin: boolean
}

const CARD = 'mb-4 overflow-hidden rounded-lg border border-line bg-surface shadow-card'
const HEAD = 'flex flex-wrap items-center justify-between gap-4 border-b border-line p-4'

/**
 * Organisation security policy.
 *
 * The requirement switch shows what turning it on would do *before* it does it.
 * Enabling blind is how a team locks itself out of its own reporting on a
 * Friday, and "we emailed everyone" is not a mitigation — the people who did
 * not read the email are exactly the ones who get locked out.
 *
 * An admin without their own second factor cannot enable the requirement at
 * all: they would be the first person it locked out, and then nobody could turn
 * it back off.
 */
export function SecurityPolicy({ orgName, members, twoFactorOn, canAdmin }: Props) {
  const [policy, setPolicy] = useState<Policy>({ requireTwoFactor: false, sessionIdleTimeout: 0 })
  const [confirming, setConfirming] = useState(false)

  const r = readiness(members, twoFactorOn)
  const blocked = !r.youAreCovered
    ? 'Turn on two-factor for your own account first — otherwise this locks you out before anyone else.'
    : undefined

  return (
    <>
      <section className={CARD} data-testid="security-policy">
        <div className={HEAD}>
          <div>
            <h2 className="text-lead font-semibold text-ink">Require two-factor authentication</h2>
            <p className="mt-1 max-w-[68ch] text-small text-ink-secondary">
              Everyone in {orgName} must have a second factor before they can sign in.
            </p>
          </div>
          <Switch checked={policy.requireTwoFactor} disabled={!canAdmin || !!blocked}
            aria-label="Require two-factor authentication"
            onChange={(e) => {
              if (e.currentTarget.checked) setConfirming(true)
              else setPolicy((p) => ({ ...p, requireTwoFactor: false }))
            }} />
        </div>

        {/* Coverage, before the decision rather than after it. */}
        <div className="border-b border-line px-4 py-3">
          <div className="flex items-center gap-3">
            <div className="h-2 flex-1 overflow-hidden rounded-full bg-grid">
              <div className="h-full rounded-full bg-good"
                style={{ width: `${r.total ? (r.covered / r.total) * 100 : 0}%` }} />
            </div>
            <span className="shrink-0 text-small text-ink-secondary tabular-nums">
              {r.covered} of {r.total} protected
            </span>
          </div>
          {r.wouldLoseAccess.length > 0 && (
            <p className="mt-2 text-small text-ink-secondary">
              Without a second factor:{' '}
              <span className="text-ink">
                {r.wouldLoseAccess.map((p) => p.name).join(', ')}
              </span>
            </p>
          )}
        </div>

        {blocked && (
          <p className="border-b border-line bg-sunken px-4 py-3 text-small text-ink-secondary">
            <strong className="text-ink">You are not protected yet.</strong> {blocked}
          </p>
        )}

        {confirming && (
          <div className="flex flex-wrap items-center gap-3 border-b border-line bg-sunken px-4 py-3">
            <p className="flex-1 text-small text-ink-secondary">
              {r.wouldLoseAccess.length === 0
                ? 'Everyone already has a second factor. Turning this on changes nothing today and applies to new members.'
                : <>This signs out <strong className="text-ink">
                    {r.wouldLoseAccess.length} {r.wouldLoseAccess.length === 1 ? 'person' : 'people'}
                  </strong> until they enrol. They keep their accounts and their work.</>}
            </p>
            <Button variant="default" size="xs" onClick={() => setConfirming(false)}>Cancel</Button>
            <Button size="xs"
              onClick={() => { setPolicy((p) => ({ ...p, requireTwoFactor: true })); setConfirming(false) }}>
              Require it
            </Button>
          </div>
        )}

        <div className="grid max-w-[420px] gap-4 p-4">
          <Field label="Sign out after inactivity" required={false}
            help="Applies to everyone in this organisation.">
            <Select allowDeselect={false} disabled={!canAdmin}
              value={String(policy.sessionIdleTimeout)}
              data={[
                { value: '0', label: 'Never' },
                { value: '60', label: 'After 1 hour' },
                { value: '480', label: 'After 8 hours' },
                { value: '10080', label: 'After 7 days' },
              ]}
              onChange={(v) => setPolicy((p) => ({ ...p, sessionIdleTimeout: Number(v) }))} />
          </Field>
        </div>
      </section>

      {/* SSO is the commercial line. Baseline account security is not — putting
          2FA behind a paid tier is the SSO tax with worse consequences. */}
      <section className={CARD}>
        <div className={HEAD}>
          <div>
            <h2 className="flex items-center gap-2 text-lead font-semibold text-ink">
              Single sign-on
              <span className="rounded-full bg-sunken px-2 py-px text-micro font-medium text-ink-secondary">
                Commercial
              </span>
            </h2>
            <p className="mt-1 max-w-[68ch] text-small text-ink-secondary">
              SAML or OIDC against your identity provider, with SCIM for joiners
              and leavers. Two-factor above is free and always will be.
            </p>
          </div>
          <Button variant="default" disabled>Contact us</Button>
        </div>
      </section>
    </>
  )
}
