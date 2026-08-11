import { useMemo, useState } from 'react'
import { Button, TextInput } from '@mantine/core'
import { Field } from '../components/form/Field'
import { Wizard, type Step } from '../components/form/Wizard'
import {
  formatSecret, makeRecoveryCodes, makeSecret, METHODS, type FactorKind,
} from '../lib/security'

interface Props {
  onDone: (kind: FactorKind, label: string) => void
  onCancel: () => void
}

const STEPS: Step[] = [
  { id: 'method', label: 'Choose a method', hint: 'How you will prove it is you' },
  { id: 'enrol', label: 'Set it up', hint: 'Link your device' },
  { id: 'verify', label: 'Prove it works', hint: 'So you cannot lock yourself out' },
  { id: 'recovery', label: 'Save recovery codes', hint: 'For when the device is gone' },
]

/**
 * Enrolling a second factor.
 *
 * The order is load-bearing. Verification comes before the factor counts, so
 * nobody can enrol into a lockout with a mistyped secret. Recovery codes come
 * last and require an explicit acknowledgement, because "I'll save them later"
 * is how a support queue fills up with account recovery.
 */
export function TwoFactorSetup({ onDone, onCancel }: Props) {
  const [step, setStep] = useState(0)
  const [kind, setKind] = useState<FactorKind | null>(null)
  const [code, setCode] = useState('')
  const [error, setError] = useState<string>()
  const [saved, setSaved] = useState(false)
  const [copied, setCopied] = useState(false)

  const secret = useMemo(() => makeSecret('cronos-dewi'), [])
  const codes = useMemo(() => makeRecoveryCodes(secret), [secret])

  function verify() {
    // Any six digits stand in for a real TOTP check.
    if (!/^\d{6}$/.test(code.trim())) {
      setError('That is not a six-digit code. Check the app and try the current one.')
      return
    }
    setError(undefined)
    setStep(3)
  }

  const canAdvance = [
    kind !== null,
    true,
    /^\d{6}$/.test(code.trim()) || kind === 'passkey',
    saved,
  ][step]!

  function next() {
    if (step === 2 && kind === 'app') return verify()
    if (step === STEPS.length - 1) return onDone(kind!, kind === 'passkey' ? 'Passkey' : 'Authenticator app')
    setStep((s) => s + 1)
  }

  return (
    <>
      <Wizard steps={STEPS} current={step} completed={step} onStep={setStep}
        onBack={() => setStep((s) => s - 1)} onNext={next} canAdvance={canAdvance}
        nextLabel={step === 3 ? 'Finish' : step === 2 ? 'Verify' : 'Continue'}>

        {step === 0 && (
          <section className="rounded-lg border border-line bg-surface p-5 shadow-card">
            <h2 className="text-lead font-semibold text-ink">How should we check it is you?</h2>
            <p className="mt-1 mb-4 max-w-[62ch] text-small text-ink-secondary">
              A second factor means a stolen password is not enough on its own.
              We do not offer text-message codes: they can be intercepted by
              taking over your phone number, which is common enough that
              offering them would be misleading.
            </p>
            <div className="grid gap-3 sm:grid-cols-2">
              {METHODS.map((m) => (
                <button key={m.kind} type="button" onClick={() => setKind(m.kind)}
                  data-testid={`method-${m.kind}`} aria-pressed={kind === m.kind}
                  className={`grid cursor-pointer gap-1 rounded-lg border bg-surface p-4 text-left
                    text-ink hover:border-accent
                    ${kind === m.kind ? 'border-accent bg-accent-wash' : 'border-line'}`}>
                  <span aria-hidden className="text-title leading-none text-accent">{m.icon}</span>
                  <span className="font-semibold">{m.label}</span>
                  <span className="text-small text-ink-secondary">{m.hint}</span>
                  <span className="mt-1 text-micro tracking-[0.04em] text-delta-good uppercase">
                    {m.strength}
                  </span>
                </button>
              ))}
            </div>
          </section>
        )}

        {step === 1 && kind === 'app' && (
          <section className="rounded-lg border border-line bg-surface p-5 shadow-card">
            <h2 className="text-lead font-semibold text-ink">Scan this with your app</h2>
            <p className="mt-1 mb-4 text-small text-ink-secondary">
              1Password, Authy, Google Authenticator — any of them work.
            </p>
            <div className="flex flex-wrap items-start gap-6">
              <QrPlaceholder seed={secret} />
              <div className="min-w-0 flex-1">
                <Field label="Or enter this key by hand" required={false}
                  help="Use this if your app cannot scan.">
                  <div className="flex items-center gap-2">
                    <code className="min-w-0 flex-1 rounded-md bg-sunken px-3 py-2 font-mono
                                     text-small break-all text-ink">
                      {formatSecret(secret)}
                    </code>
                    <Button variant="default" size="xs"
                      onClick={() => { void navigator.clipboard?.writeText(secret); setCopied(true) }}>
                      {copied ? 'Copied' : 'Copy'}
                    </Button>
                  </div>
                </Field>
              </div>
            </div>
          </section>
        )}

        {step === 1 && kind === 'passkey' && (
          <section className="rounded-lg border border-line bg-surface p-5 shadow-card">
            <h2 className="text-lead font-semibold text-ink">Create a passkey</h2>
            <p className="mt-1 mb-4 max-w-[62ch] text-small text-ink-secondary">
              Your browser will ask for your fingerprint, face or security key. The
              key never leaves your device, and it only works on this site — which
              is why a passkey cannot be phished.
            </p>
            <Button>Create passkey</Button>
          </section>
        )}

        {step === 2 && (
          <section className="rounded-lg border border-line bg-surface p-5 shadow-card">
            <h2 className="text-lead font-semibold text-ink">Enter the current code</h2>
            <p className="mt-1 mb-4 max-w-[62ch] text-small text-ink-secondary">
              This proves the setup worked. Nothing is switched on until it does —
              otherwise a mistyped key would lock you out of your own account.
            </p>
            <Field label="Six-digit code" error={error}>
              <TextInput value={code} inputMode="numeric" maxLength={6} w={180}
                placeholder="123456" aria-label="Six-digit code"
                classNames={{ input: 'font-mono text-lead tracking-[0.3em]' }}
                onChange={(e) => { setCode(e.currentTarget.value); setError(undefined) }} />
            </Field>
          </section>
        )}

        {step === 3 && (
          <section className="rounded-lg border border-line bg-surface p-5 shadow-card">
            <h2 className="text-lead font-semibold text-ink">Save these somewhere safe</h2>
            <p className="mt-1 mb-4 max-w-[62ch] text-small text-ink-secondary">
              Each code works once, and gets you in if you lose the device.
              <strong className="text-ink"> This is the only time they are shown.</strong>{' '}
              An administrator can reset your second factor, but that is recorded
              in the audit log.
            </p>
            <ul className="grid grid-cols-2 gap-2 rounded-lg bg-sunken p-4 font-mono text-small
                           sm:grid-cols-2">
              {codes.map((c) => <li key={c} className="text-ink">{c}</li>)}
            </ul>
            <div className="mt-4 flex flex-wrap items-center gap-2">
              <Button variant="default" size="xs"
                onClick={() => void navigator.clipboard?.writeText(codes.join('\n'))}>
                Copy all
              </Button>
              <Button variant="default" size="xs">Download</Button>
            </div>
            <label className="mt-4 flex cursor-pointer items-start gap-2 text-small text-ink">
              <input type="checkbox" checked={saved} className="mt-0.5"
                onChange={(e) => setSaved(e.currentTarget.checked)} />
              I have saved these codes somewhere I can reach without this account.
            </label>
          </section>
        )}
      </Wizard>

      <button type="button" onClick={onCancel}
        className="mt-4 cursor-pointer p-0 text-small text-ink-muted underline">
        Cancel
      </button>
    </>
  )
}

/**
 * Stands in for a real QR until the engine issues one. Deterministic from the
 * secret so it looks like a code rather than noise, and stable across renders.
 */
function QrPlaceholder({ seed }: { seed: string }) {
  const cells = useMemo(() => {
    let h = 0
    for (const c of seed) h = (h * 31 + c.charCodeAt(0)) >>> 0
    return Array.from({ length: 441 }, (_, i) => {
      const r = Math.floor(i / 21)
      const c = i % 21
      const finder = (r < 7 && c < 7) || (r < 7 && c > 13) || (r > 13 && c < 7)
      if (finder) {
        const rr = r > 13 ? r - 14 : r
        const cc = c > 13 ? c - 14 : c
        const ring = Math.max(Math.abs(rr - 3), Math.abs(cc - 3))
        return ring === 1 ? 0 : 1
      }
      h = (h * 1103515245 + 12345) >>> 0
      return h % 100 < 45 ? 1 : 0
    })
  }, [seed])

  return (
    <svg viewBox="0 0 21 21" role="img" aria-label="Setup QR code"
      className="size-[168px] shrink-0 rounded-md bg-white p-2 ring-1 ring-line">
      {cells.map((on, i) => on
        ? <rect key={i} x={i % 21} y={Math.floor(i / 21)} width={1} height={1} fill="#0b0b0b" />
        : null)}
    </svg>
  )
}
