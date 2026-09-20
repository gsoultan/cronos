import { useEffect, useState } from 'react'
import { Button, PasswordInput, TextInput } from '@mantine/core'
import { Brand } from '../components/Brand'
import { codeProblem, emailProblem, passwordProblem } from '../lib/signin'
import {
  ApiError, askForReset, signIn, signInMethods, ssoStart, type SignInMethods,
} from '../lib/api'

/**
 * Sign in.
 *
 * Shown whenever a server is configured and nobody has a session — including
 * when one expires mid-use, because the alternative is an error page that
 * says "unauthorised" and offers no way out of it.
 *
 * One message for every failure, which is the server's. Telling "no such
 * account" apart from "wrong password" is how somebody learns which addresses
 * are registered, and that is worth more to a phisher than to anyone honest.
 *
 * Two columns at lg: what this is on the left, the form on the right. The
 * panel is the only thing the split adds — the form is the same form, and
 * below lg it is the whole page, as it was.
 */
export function SignInPage({ onSignedIn }: { onSignedIn: () => void }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  /* Shown only after the password has been accepted, which is what stops this
     page being a way to learn which accounts have a second factor. */
  const [needsCode, setNeedsCode] = useState(false)
  const [code, setCode] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  /* Which fields somebody is finished with. A message under a field they are
     still typing into tells them they are wrong before they have finished
     being right, so nothing appears until they leave the field or press the
     button. Once it has appeared it clears itself on the keystroke that fixes
     it, which is the half that makes it feel like help rather than nagging. */
  const [touched, setTouched] = useState({ email: false, password: false, code: false })
  const touch = (field: keyof typeof touched) => setTouched((t) => ({ ...t, [field]: true }))

  /* Recomputed every render rather than stored. Two copies of "is this field
     all right" is one copy that goes stale. */
  const problems = {
    email: emailProblem(email),
    password: passwordProblem(password),
    code: needsCode ? codeProblem(code) : null,
  }
  const incomplete = !!(problems.email || problems.password || problems.code)

  /* What this deployment lets people in with. Asked rather than assumed: a
     button for a directory nobody configured leads to an error, and a
     deployment that has one and does not show it sends everybody to a password
     form they may not have a password for. */
  const [methods, setMethods] = useState<SignInMethods | null>(null)
  useEffect(() => { void signInMethods().then(setMethods) }, [])

  /* The identity provider sends people back here with a complaint in the
     query, because a page of JSON is not an answer to somebody who clicked a
     button. */
  /* Asking is its own small state, not folded into the sign-in error: "a link
     is on its way" is not a failure to sign in, and rendering it in the same
     red box as a wrong password says the opposite of what it means. */
  const [sending, setSending] = useState(false)
  const [sent, setSent] = useState(false)

  const [ssoError] = useState(() =>
    new URLSearchParams(globalThis.location?.search ?? '').get('sso_error'))

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    /* Pressing the button is somebody saying they are done with every field,
       so this is where the empty ones start saying what they are missing —
       rather than the form reaching the server to come back with "check your
       details", which is the sentence a wrong password gets too and so says
       nothing about the field that is simply blank. */
    setTouched({ email: true, password: true, code: true })
    if (incomplete) return

    setBusy(true)
    setError(null)
    try {
      const out = await signIn(email, password, code || undefined)
      if (out.factorRequired) {
        /* The password was right and this account has a second factor. Not an
           error — nothing was refused — so the field appears and the message
           says what to do rather than what went wrong. */
        setNeedsCode(true)
        setError(null)
        return
      }
      onSignedIn()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not reach the server.')
      // The app has moved on by now, so the old digits are stale whatever went
      // wrong. Clearing saves somebody pressing sign-in twice on the same code.
      setCode('')
      /* And the field going empty is this code's doing, not a mistake somebody
         made, so it goes back to unmarked. Stacking "enter the code" under the
         server's own sentence is two complaints about one event. */
      setTouched((t) => ({ ...t, code: false }))
    } finally {
      setBusy(false)
    }
  }

  /* The address from the field, because there is nowhere else to get it and a
     second form for one input is a page nobody needs. Errors are shown, but
     the only one the server sends is "this deployment cannot send email" —
     everything else is deliberately indistinguishable, including an address
     that has no account. */
  async function forgot() {
    setSending(true)
    setError(null)
    try {
      await askForReset(email.trim())
      setSent(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not ask for a link.')
    } finally {
      setSending(false)
    }
  }

  return (
    <main className="grid min-h-screen bg-plane lg:grid-cols-2">
      {/*
        The half that says what this is.

        Hidden outright below lg rather than stacked above the form: on a phone
        a panel above the fields is a pitch to scroll past, and most people
        arriving here clicked a report link and were bounced. They want the
        form. Nothing lives in here that is not also somewhere else.
      */}
      <aside className="relative hidden flex-col justify-center bg-seq-700 p-12
                        lg:flex xl:p-16">
        {/*
          The corner mark, out of the flow so the statement can sit on the
          panel's optical centre and line up with the form beside it.

          This is also the slot a customer's own logo belongs in, and it is
          empty of one deliberately. Branding in this app is per organisation
          (WorkspaceContext keys it by org id, after the leak that put one
          organisation's logo in another's settings), and an organisation is
          something the token says — there isn't one yet on this page. A
          deployment can hold more than one, so the only honest logo here is a
          deployment-level one, served unauthenticated the way /v1/auth/methods
          already is. When that exists it takes this position and the cronos
          lockup demotes to a line at the foot of the panel.
        */}
        <Brand inherit className="brand-draw animate-rise absolute top-12 left-12
                                  text-seq-100 xl:top-16 xl:left-16" />

        <div>
          <p className="animate-rise max-w-[18ch] text-display font-semibold
                        leading-tight tracking-[-0.02em] text-seq-100
                        [animation-delay:120ms]">
            The same report, every time it is asked for.
          </p>
          <p className="animate-rise mt-5 max-w-[44ch] text-lead leading-relaxed
                        text-seq-250 [animation-delay:220ms]">
            One definition, delivered on a schedule, to the people who need it —
            and the same numbers whoever opens it.
          </p>
        </div>
      </aside>

      <div className="grid place-items-center p-4">
        {/*
          noValidate. The fields still declare `required`, so assistive
          technology and password managers still read them as required, but the
          browser's own bubble is suppressed in favour of the messages below.
          The bubble covers one field at a time, disappears on the next click,
          is not in the page and so reaches no screen reader — and this form
          often needs to say two things at once.
        */}
        <form onSubmit={submit} noValidate data-testid="sign-in"
          className="w-full max-w-[380px] rounded-lg border border-line bg-surface p-8
                     shadow-card lg:border-0 lg:bg-transparent lg:p-0 lg:shadow-none">
          {/* The panel carries the mark at lg, so the card stops repeating it. */}
          <div className="mb-6 flex justify-center lg:hidden"><Brand /></div>

          {ssoError && (
            <p role="alert" data-testid="sso-error"
              className="mb-4 rounded-md border border-serious/30 bg-serious/10 px-3 py-2
                         text-small text-ink">
              {ssoError}.
            </p>
          )}

          {methods?.sso && (
            <div className="mb-6 grid gap-4">
              <Button component="a" data-testid="sso-button" fullWidth variant="default"
                href={ssoStart(globalThis.location?.pathname ?? '/')}>
                Sign in with single sign-on
              </Button>

              {/* Only when there is a choice. A deployment that has removed
                  passwords should not show a form nobody can use, and one that
                  has both should not make either look like the wrong door. */}
              {methods.password && (
                <div className="flex items-center gap-3 text-caption text-ink-muted">
                  <span className="h-px flex-1 bg-line" />or<span className="h-px flex-1 bg-line" />
                </div>
              )}
            </div>
          )}

          <h1 className="mb-1 text-center text-title font-semibold text-ink">Sign in</h1>
          <p className="mb-6 text-center text-small text-ink-secondary">
            To the reports in your project.
          </p>

          <div className={`grid gap-4 ${methods && !methods.password ? 'hidden' : ''}`}>
            <TextInput label="Email" type="email" required autoFocus
              autoComplete="username" value={email} data-testid="email"
              error={touched.email ? problems.email : null}
              onBlur={() => touch('email')}
              onChange={(e) => setEmail(e.currentTarget.value)} />
            <PasswordInput label="Password" required
              autoComplete="current-password" value={password} data-testid="password"
              error={touched.password ? problems.password : null}
              onBlur={() => touch('password')}
              onChange={(e) => setPassword(e.currentTarget.value)} />

            {needsCode && (
              <TextInput label="Code from your authenticator app" required autoFocus
                data-testid="factor-code" value={code}
                /* one-time-code lets a phone offer the digits straight from the
                   notification, and inputMode brings up the number pad. */
                autoComplete="one-time-code" inputMode="numeric" maxLength={11}
                placeholder="123456"
                description="Or one of your recovery codes, if you no longer have the app."
                error={touched.code ? problems.code : null}
                onBlur={() => touch('code')}
                onChange={(e) => setCode(e.currentTarget.value)} />
            )}

            {error && (
              <p data-testid="sign-in-error" role="alert"
                className="rounded-md bg-serious/10 px-3 py-2 text-small text-ink">
                {error}
              </p>
            )}

            {/* Never disabled on an incomplete form. A button that does nothing
                and does not say why is a worse dead end than a message: the
                press is what asks the question, so the press has to answer it. */}
            <Button type="submit" loading={busy} data-testid="submit" fullWidth>
              Sign in
            </Button>

            {/*
              Only where a link can actually be sent. On a deployment with no mail
              relay this is absent rather than present and apologetic: an offer of
              help that turns into "not configured" is a worse place to leave
              somebody who is already locked out than no offer at all.

              Below the button, not beside the password field. A person who can
              sign in should not be reading it, and a person who cannot has
              already tried.
            */}
            {methods?.reset && (
              <div className="text-center">
                {sent ? (
                  <p data-testid="reset-sent" className="text-small text-ink-secondary">
                    If that address has an account, a link is on its way. It works
                    once and expires in an hour.
                  </p>
                ) : (
                  <button type="button" data-testid="forgot"
                    className="cursor-pointer text-small text-ink-muted underline
                               disabled:cursor-default disabled:opacity-60"
                    disabled={sending || email.trim() === ''}
                    title={email.trim() === '' ? 'Enter your email address first' : undefined}
                    onClick={() => { void forgot() }}>
                    Forgot your password?
                  </button>
                )}
              </div>
            )}
          </div>
        </form>
      </div>
    </main>
  )
}
