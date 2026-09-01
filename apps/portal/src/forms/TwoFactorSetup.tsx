import { useEffect, useRef, useState } from 'react'
import { Button, TextInput } from '@mantine/core'
import { Field } from '../components/form/Field'
import { QrCode } from '../components/QrCode'
import { Wizard, type Step } from '../components/form/Wizard'
import { ApiError, confirmFactor, startFactor } from '../lib/api'
import { formatSecret } from '../lib/security'

interface Props {
  onDone: () => void
  onCancel: () => void
}

const STEPS: Step[] = [
  { id: 'enrol', label: 'Set it up', hint: 'Link your authenticator app' },
  { id: 'verify', label: 'Prove it works', hint: 'So you cannot lock yourself out' },
  { id: 'recovery', label: 'Save recovery codes', hint: 'For when the device is gone' },
]

/**
 * Enrolling a second factor.
 *
 * The order is load-bearing and, until now, was the only part that was real.
 * Verification came before the factor counted — except the check accepted any
 * six digits, the QR code was noise with finder squares drawn in the corners,
 * and the recovery codes were generated in the browser and stored nowhere. An
 * account could complete this wizard and end up marked as protected by a secret
 * that existed in no authenticator app anywhere.
 *
 * Every step now talks to the server. The secret is minted there and comes back
 * once; the code is checked against that exact secret; the recovery codes are
 * issued at confirmation and stored as hashes. Nothing is switched on until a
 * real code proves the app holds the real secret.
 *
 * The method chooser is gone with the passkey option it offered. WebAuthn is
 * genuinely the stronger factor and is not built — a button that says
 * "Strongest — cannot be phished" and does nothing is the same class of thing
 * this change exists to remove.
 */
export function TwoFactorSetup({ onDone, onCancel }: Props) {
  const [step, setStep] = useState(0)
  const [enrolment, setEnrolment] = useState<{ secret: string; uri: string } | null>(null)
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState<string[]>([])
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [copied, setCopied] = useState(false)

  /*
   * Started as the page opens, because the QR cannot be drawn without it. An
   * enrolment that is begun and abandoned leaves an unconfirmed row that
   * protects nothing and is replaced by the next attempt.
   *
   * Started *once*, which the `live` flag alone did not achieve. Beginning an
   * enrolment is not an idempotent thing to do: each call replaces the pending
   * secret. StrictMode runs an effect twice on purpose to find exactly this,
   * and two starts in flight at once race — the server keeps whichever it
   * finishes last, the page shows whichever it is allowed to set, and about
   * half the time those are different secrets. Then every code from the QR
   * just scanned is refused as wrong, with no way to tell why.
   *
   * What is held is the promise, not a flag saying one was sent. A flag is the
   * obvious version and it is wrong in a way that takes a while to see: the
   * first mount sends the request and its cleanup marks itself dead, the second
   * mount finds the flag set and sends nothing, and so nobody is left listening
   * for the answer. The QR never appears at all. Sharing the promise means one
   * request and whichever mount is alive when it lands sets the state.
   *
   * The `live` flag is still needed and does a different job: it stops a
   * response arriving after the wizard has closed from setting state on
   * something that is gone.
   *
   * The other fix would be for /v1/auth/factor/start to hand back the pending
   * secret it already has instead of minting a new one, which is also better
   * for somebody who refreshes the page mid-enrolment. That is a change to what
   * a route means and is not made here.
   */
  const started = useRef<ReturnType<typeof startFactor> | null>(null)
  useEffect(() => {
    let live = true
    started.current ??= startFactor()
    started.current
      .then((out) => { if (live) setEnrolment(out) })
      .catch((err: unknown) => {
        if (live) setError(err instanceof ApiError ? err.message : 'Could not start enrolment.')
      })
    return () => { live = false }
  }, [])

  async function verify() {
    setBusy(true)
    setError(undefined)
    try {
      const out = await confirmFactor(code.trim())
      setCodes(out.recoveryCodes)
      setStep(2)
    } catch (err) {
      // The server's own sentence: it is the only party that knows whether this
      // was a wrong code, a stale one, or a code already used.
      setError(err instanceof ApiError ? err.message : 'Could not check that code.')
    } finally {
      setBusy(false)
    }
  }

  const canAdvance = [enrolment !== null, /^\d{6}$/.test(code.trim()), saved][step]!

  function next() {
    if (step === 1) return void verify()
    if (step === STEPS.length - 1) return onDone()
    setStep((s) => s + 1)
  }

  return (
    <>
      <Wizard steps={STEPS} current={step} completed={step}
        /* No jumping back to a step that has already happened on the server.
           Re-running enrolment after confirming would mint a second secret and
           strand the app that holds the first. */
        onStep={(to) => { if (to < step && step < 2) setStep(to) }}
        onBack={() => setStep((s) => s - 1)} onNext={next}
        canAdvance={canAdvance && !busy}
        nextLabel={step === 2 ? 'Finish' : step === 1 ? 'Verify' : 'Continue'}>

        {step === 0 && (
          <section className="rounded-lg border border-line bg-surface p-5 shadow-card">
            <h2 className="text-lead font-semibold text-ink">Scan this with your app</h2>
            <p className="mt-1 mb-4 max-w-[62ch] text-small text-ink-secondary">
              1Password, Authy, Google Authenticator — any of them work. We do not
              offer text-message codes: they can be intercepted by taking over
              your phone number, which is common enough that offering them would
              be misleading.
            </p>

            {!enrolment && !error && (
              <p className="text-small text-ink-muted">Preparing…</p>
            )}
            {error && !enrolment && (
              <p role="alert" className="text-small text-danger">{error}</p>
            )}

            {enrolment && (
              <div className="flex flex-wrap items-start gap-6">
                <QrCode text={enrolment.uri} label="Scan to add cronos to your authenticator app" />
                <div className="min-w-0 flex-1">
                  <Field label="Or enter this key by hand" required={false}
                    help="Use this if your app cannot scan.">
                    <div className="flex items-center gap-2">
                      <code className="min-w-0 flex-1 rounded-md bg-sunken px-3 py-2 font-mono
                                       text-small break-all text-ink">
                        {formatSecret(enrolment.secret)}
                      </code>
                      <Button variant="default" size="xs"
                        onClick={() => {
                          void navigator.clipboard?.writeText(enrolment.secret)
                          setCopied(true)
                        }}>
                        {copied ? 'Copied' : 'Copy'}
                      </Button>
                    </div>
                  </Field>
                </div>
              </div>
            )}
          </section>
        )}

        {step === 1 && (
          <section className="rounded-lg border border-line bg-surface p-5 shadow-card">
            <h2 className="text-lead font-semibold text-ink">Enter the current code</h2>
            <p className="mt-1 mb-4 max-w-[62ch] text-small text-ink-secondary">
              This proves the setup worked, and it is checked against the key the
              server stored. Nothing is switched on until it passes — otherwise a
              mistyped key would lock you out of your own account.
            </p>
            <Field label="Six-digit code" error={error}>
              <TextInput value={code} inputMode="numeric" maxLength={6} w={180}
                placeholder="123456" aria-label="Six-digit code" autoFocus
                autoComplete="one-time-code" data-testid="factor-verify"
                classNames={{ input: 'font-mono text-lead tracking-[0.3em]' }}
                onChange={(e) => { setCode(e.currentTarget.value); setError(undefined) }} />
            </Field>
          </section>
        )}

        {step === 2 && (
          <section className="rounded-lg border border-line bg-surface p-5 shadow-card">
            <h2 className="text-lead font-semibold text-ink">Save these somewhere safe</h2>
            <p className="mt-1 mb-4 max-w-[62ch] text-small text-ink-secondary">
              Each code works once, and gets you in if you lose the device.
              <strong className="text-ink"> This is the only time they are shown.</strong>{' '}
              They are stored as hashes, so nobody here can read them back to you —
              including us.
            </p>
            <ul data-testid="recovery-codes"
              className="grid grid-cols-2 gap-2 rounded-lg bg-sunken p-4 font-mono text-small">
              {codes.map((c) => <li key={c} className="text-ink">{c}</li>)}
            </ul>
            <div className="mt-4 flex flex-wrap items-center gap-2">
              <Button variant="default" size="xs"
                onClick={() => void navigator.clipboard?.writeText(codes.join('\n'))}>
                Copy all
              </Button>
              <Button variant="default" size="xs"
                onClick={() => download('cronos-recovery-codes.txt', codes.join('\n') + '\n')}>
                Download
              </Button>
            </div>
            <label className="mt-4 flex cursor-pointer items-start gap-2 text-small text-ink">
              <input type="checkbox" checked={saved} className="mt-0.5"
                data-testid="codes-saved"
                onChange={(e) => setSaved(e.currentTarget.checked)} />
              I have saved these codes somewhere I can reach without this account.
            </label>
          </section>
        )}
      </Wizard>

      {step < 2 && (
        <button type="button" onClick={onCancel}
          className="mt-4 cursor-pointer p-0 text-small text-ink-muted underline">
          Cancel
        </button>
      )}
    </>
  )
}

/**
 * Hands the codes to the browser as a file.
 *
 * A real download rather than a button that did nothing, which is what was here
 * before. Somebody who reaches for it is doing the right thing, and the moment
 * they find it does not work is the moment they decide to skip the step.
 */
function download(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }))
  const link = document.createElement('a')
  link.href = url
  link.download = name
  link.click()
  URL.revokeObjectURL(url)
}
