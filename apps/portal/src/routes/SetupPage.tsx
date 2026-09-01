import { useState } from 'react'
import { Button, PasswordInput, TextInput } from '@mantine/core'
import { Brand } from '../components/Brand'
import { ApiError, setUp } from '../lib/api'

/**
 * The first run.
 *
 * A fresh install has a database, a server and no accounts, and before this the
 * only way to make the first one was `cronos-user` on the machine — fine for
 * somebody with shell access, impossible for anybody handed a URL.
 *
 * Shown only when the server says it is needed, which it says only while no
 * account exists at all. The moment this succeeds the endpoint behind it closes
 * for good, so the page cannot be reached twice and nothing reopens it.
 */
export function SetupPage({ onDone }: { onDone: () => void }) {
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [org, setOrg] = useState('')
  const [project, setProject] = useState('')
  const [password, setPassword] = useState('')
  const [again, setAgain] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const tooShort = password.length > 0 && password.length < 12
  const mismatch = again.length > 0 && again !== password
  const ready = email.includes('@') && org.trim() !== '' && project.trim() !== '' &&
    password.length >= 12 && again === password

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!ready || busy) return

    setBusy(true)
    setError(null)
    try {
      await setUp({ email, name, password, org, project })
      onDone()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not reach the server.')
      setBusy(false)
    }
  }

  return (
    <main className="grid min-h-screen place-items-center bg-canvas p-4">
      <form onSubmit={(e) => void submit(e)} data-testid="setup"
        className="w-full max-w-[440px] rounded-lg border border-line bg-surface p-8 shadow-card">
        <div className="mb-6 flex justify-center"><Brand /></div>

        <h1 className="text-title font-semibold text-ink">Set up cronos</h1>
        <p className="mt-2 mb-6 text-small text-ink-secondary">
          Nobody has an account here yet, so this page is open. It closes as soon
          as you finish, and the account you create administers the deployment.
        </p>

        <div className="grid gap-4">
          <TextInput label="Your email" type="email" required autoFocus
            autoComplete="username" value={email} data-testid="setup-email"
            description="What you will sign in with."
            onChange={(e) => setEmail(e.currentTarget.value)} />

          <TextInput label="Your name" required={false} value={name}
            autoComplete="name" data-testid="setup-name"
            onChange={(e) => setName(e.currentTarget.value)} />

          <div className="grid gap-4 sm:grid-cols-2">
            <TextInput label="Organisation" required value={org} data-testid="setup-org"
              placeholder="Acme Logistics"
              onChange={(e) => setOrg(e.currentTarget.value)} />
            <TextInput label="First project" required value={project} data-testid="setup-project"
              placeholder="Finance"
              onChange={(e) => setProject(e.currentTarget.value)} />
          </div>
          {/* These become identifiers rather than labels — half of every
              tenancy check, and part of a path when definitions live on disk —
              so the server reduces them and the form says so rather than
              letting somebody discover it afterwards. */}
          <p className="-mt-2 text-caption text-ink-muted">
            Both become identifiers: letters, digits and hyphens. &ldquo;Acme
            Logistics&rdquo; is stored as <code>acme-logistics</code>.
          </p>

          <PasswordInput label="Password" required value={password}
            autoComplete="new-password" data-testid="setup-password"
            error={tooShort ? 'At least 12 characters.' : undefined}
            description="At least 12 characters. This account administers everything."
            onChange={(e) => setPassword(e.currentTarget.value)} />

          <PasswordInput label="Password again" required value={again}
            autoComplete="new-password" data-testid="setup-password-again"
            error={mismatch ? 'These do not match.' : undefined}
            onChange={(e) => setAgain(e.currentTarget.value)} />

          {error && (
            <p role="alert" data-testid="setup-error"
              className="rounded-md bg-serious/10 px-3 py-2 text-small text-ink">
              {error}
            </p>
          )}

          <Button type="submit" fullWidth disabled={!ready} loading={busy}
            data-testid="setup-submit">
            Create the first account
          </Button>
        </div>
      </form>
    </main>
  )
}
