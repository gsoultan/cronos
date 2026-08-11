import { useState } from 'react'
import { Button, PasswordInput, TextInput } from '@mantine/core'
import { Brand } from '../components/Brand'
import { ApiError, signIn } from '../lib/api'

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
 */
export function SignInPage({ onSignedIn }: { onSignedIn: () => void }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await signIn(email, password)
      onSignedIn()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not reach the server.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="grid min-h-screen place-items-center bg-canvas p-4">
      <form onSubmit={submit} data-testid="sign-in"
        className="w-full max-w-[380px] rounded-lg border border-line bg-surface p-8 shadow-card">
        <div className="mb-6 flex justify-center"><Brand /></div>

        <h1 className="mb-1 text-center text-title font-semibold text-ink">Sign in</h1>
        <p className="mb-6 text-center text-small text-ink-secondary">
          To the reports in your project.
        </p>

        <div className="grid gap-4">
          <TextInput label="Email" type="email" required autoFocus
            autoComplete="username" value={email} data-testid="email"
            onChange={(e) => setEmail(e.currentTarget.value)} />
          <PasswordInput label="Password" required
            autoComplete="current-password" value={password} data-testid="password"
            onChange={(e) => setPassword(e.currentTarget.value)} />

          {error && (
            <p data-testid="sign-in-error" role="alert"
              className="rounded-md bg-serious/10 px-3 py-2 text-small text-ink">
              {error}
            </p>
          )}

          <Button type="submit" loading={busy} data-testid="submit" fullWidth>
            Sign in
          </Button>
        </div>
      </form>
    </main>
  )
}
