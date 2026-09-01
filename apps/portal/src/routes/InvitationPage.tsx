import { useEffect, useState } from 'react'
import { Button, PasswordInput } from '@mantine/core'
import { Brand } from '../components/Brand'
import { acceptInvitation, ApiError, describeInvitation } from '../lib/api'

/**
 * Setting a password from an invitation.
 *
 * The one page in this portal that works with no session, because the person
 * reading it does not have an account yet — that is the whole point. Its only
 * credential is the secret from the link, which arrives in the fragment.
 *
 * It says who the invitation is for before asking for anything. A page that
 * opens with an unlabelled password box and no name on it is indistinguishable
 * from a phishing page, and the difference somebody can actually check is
 * whether it already knows who they are and who invited them.
 */
export function InvitationPage() {
  const [secret] = useState(readSecret)
  const [invited, setInvited] = useState<Invited | null>(null)
  const [problem, setProblem] = useState<string | null>(null)
  const [password, setPassword] = useState('')
  const [again, setAgain] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!secret) {
      setProblem('This link is incomplete. Ask whoever invited you for a new one.')
      return
    }
    let live = true
    describeInvitation(secret)
      .then((who) => { if (live) setInvited(who) })
      .catch((err: unknown) => {
        // The server's own sentence. It is the only party that knows whether
        // this was expired, spent or never issued — and it deliberately says
        // the same thing for all three.
        if (live) setProblem(err instanceof ApiError ? err.message : 'This link does not work.')
      })
    return () => { live = false }
  }, [secret])

  const tooShort = password.length > 0 && password.length < 12
  const mismatch = again.length > 0 && again !== password
  const ready = password.length >= 12 && again === password && !busy

  async function accept(e: React.FormEvent) {
    e.preventDefault()
    if (!ready || !secret) return

    setBusy(true)
    setProblem(null)
    try {
      await acceptInvitation(secret, password)
      // The session came back with the acceptance, so there is no sign-in page
      // in between. A full navigation rather than a route change: it clears
      // the fragment, and the shell reads the session once at mount.
      globalThis.location?.assign('/')
    } catch (err) {
      setProblem(err instanceof ApiError ? err.message : 'Could not set your password.')
      setBusy(false)
    }
  }

  return (
    <main className="grid min-h-screen place-items-center bg-canvas px-4">
      <div className="w-full max-w-[400px]">
        <div className="mb-6 flex justify-center"><Brand /></div>

        {problem && !invited && (
          <div className="rounded-lg border border-line bg-surface p-6 text-center shadow-card">
            <h1 className="text-lead font-semibold text-ink">This invitation cannot be used</h1>
            <p className="mt-2 text-small text-ink-secondary">{problem}</p>
            <Button component="a" href="/" variant="default" className="mt-4">
              Go to sign in
            </Button>
          </div>
        )}

        {!problem && !invited && (
          <p className="text-center text-small text-ink-secondary">Checking your invitation…</p>
        )}

        {invited && (
          <form onSubmit={(e) => void accept(e)}
            className="rounded-lg border border-line bg-surface p-6 shadow-card">
            <h1 className="text-lead font-semibold text-ink">
              {invited.name ? `Welcome, ${invited.name}` : 'Welcome'}
            </h1>
            <p className="mt-2 text-small text-ink-secondary">
              {invited.invitedBy ? <><strong className="text-ink">{invited.invitedBy}</strong> invited you</> : 'You have been invited'}
              {' '}to <strong className="text-ink">{invited.project}</strong> as {invited.role}.
              Choose a password for <strong className="text-ink">{invited.email}</strong>.
            </p>

            <PasswordInput className="mt-4" label="Password" required autoFocus
              autoComplete="new-password" value={password}
              onChange={(e) => setPassword(e.currentTarget.value)}
              error={tooShort ? 'At least 12 characters.' : undefined}
              description="At least 12 characters. Nobody else will ever see it." />

            <PasswordInput className="mt-3" label="Password again" required
              autoComplete="new-password" value={again}
              onChange={(e) => setAgain(e.currentTarget.value)}
              error={mismatch ? 'These do not match.' : undefined} />

            {problem && (
              <p role="alert" className="mt-3 text-small text-danger">{problem}</p>
            )}

            <Button type="submit" fullWidth className="mt-5" disabled={!ready} loading={busy}>
              Set password and sign in
            </Button>
          </form>
        )}
      </div>
    </main>
  )
}

interface Invited {
  email: string
  name?: string
  org: string
  project: string
  role: string
  invitedBy?: string
}

/**
 * Takes the secret out of the fragment and off the address bar.
 *
 * The fragment because a browser never sends it to a server: in the query
 * string this secret would be in the portal's access log, its CDN's, and the
 * Referer of whatever this page loads next. Removed from the URL in the same
 * breath so it does not survive a screenshot, a bookmark or a shoulder.
 */
function readSecret(): string {
  const hash = globalThis.location?.hash ?? ''
  const found = new URLSearchParams(hash.replace(/^#/, '')).get('secret')
  if (found) {
    globalThis.history?.replaceState(null, '', globalThis.location.pathname)
  }
  return found ?? ''
}
