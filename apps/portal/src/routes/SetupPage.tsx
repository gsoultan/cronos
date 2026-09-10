import { useState } from 'react'
import { Button, PasswordInput, TextInput } from '@mantine/core'
import { Brand } from '../components/Brand'
import { ApiError, configureFirstRun, setUp, waitForRestart, type SetupState } from '../lib/api'

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
export function SetupPage({ onDone, state }: {
  onDone: () => void
  /** What the server said when asked. Absent means the account-only flow. */
  state?: SetupState
}) {
  /*
     A server with no configuration at all is a different page.

     It needs a token from the machine, it collects where the data lives, and
     it restarts rather than signing anybody in — so it is its own component
     instead of a pile of conditionals inside this one.
  */
  if (state?.unconfigured) {
    return <FirstRunPage onDone={onDone} state={state} />
  }
  return <FirstAccountPage onDone={onDone} />
}

function FirstAccountPage({ onDone }: { onDone: () => void }) {
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

/**
 * Configuring a deployment that has none.
 *
 * The other first run, and the one with a lock on it. This server has no
 * signing key: it is serving this endpoint and the probes, and nothing else.
 * Whoever finishes this form decides what the deployment is and who
 * administers it, so it asks for a token that only somebody on the machine can
 * read — see boot/setup.go for why the account-only flow needs no such thing.
 *
 * It signs nobody in. The server writes its configuration and restarts, so
 * there is no session to hand back; the page waits for it to come up and sends
 * you to sign in as the account you just described.
 */
function FirstRunPage({ onDone, state }: { onDone: () => void; state: SetupState }) {
  const [token, setToken] = useState('')
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [again, setAgain] = useState('')
  const [org, setOrg] = useState('')
  const [project, setProject] = useState('')
  const [storeDsn, setStoreDsn] = useState('')
  const [busy, setBusy] = useState(false)
  const [waiting, setWaiting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fixed = state.fixed ?? []
  const pinned = (name: string) => fixed.includes(name)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (password !== again) {
      setError('Those two passwords are different.')
      return
    }
    setBusy(true)
    setError(null)
    try {
      await configureFirstRun({
        token: token.trim(), email, name, password, org, project,
        // Blank means "let the server keep its default", which for the store
        // is no store at all — a file-backed deployment, which is a legitimate
        // way to run cronos and not an incomplete answer.
        storeDsn: storeDsn.trim() || undefined,
        storeDriver: storeDsn.trim() ? driverFor(storeDsn) : undefined,
      })
      // It is restarting. Waiting rather than declaring success immediately:
      // the second boot runs migrations, and telling somebody to sign in
      // before it can serve them is how the first thing they see is an error.
      setWaiting(true)
      await waitForRestart()
      onDone()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not reach the server.')
      setBusy(false)
    }
  }

  if (waiting) {
    return (
      <main className="grid min-h-screen place-items-center bg-canvas p-4">
        <div className="w-full max-w-[440px] rounded-lg border border-line bg-surface p-8 text-center shadow-card">
          <div className="mb-6 flex justify-center"><Brand /></div>
          <h1 className="text-title font-semibold text-ink">Configured</h1>
          <p className="mt-2 text-small text-ink-secondary">
            cronos is restarting into its new configuration. This page will move
            on when it answers.
          </p>
        </div>
      </main>
    )
  }

  return (
    <main className="grid min-h-screen place-items-center bg-canvas p-4">
      <form onSubmit={(e) => void submit(e)} data-testid="first-run"
        className="w-full max-w-[520px] rounded-lg border border-line bg-surface p-8 shadow-card">
        <div className="mb-6 flex justify-center"><Brand /></div>

        <h1 className="text-title font-semibold text-ink">Configure cronos</h1>
        <p className="mt-2 mb-6 text-small text-ink-secondary">
          This deployment has no configuration. What you set here is written to{' '}
          <code className="font-mono text-ink">{state.config ?? 'the server'}</code>,
          and cronos restarts into it. The signing key is generated for you —
          it is the root of trust for every token this deployment issues, and
          nobody should be choosing a memorable one.
        </p>

        <div className="grid gap-4">
          <TextInput label="Setup token" required autoFocus value={token}
            data-testid="first-run-token" className="font-mono"
            description="From the file named in the server's log — it proves you are on this machine."
            onChange={(e) => setToken(e.currentTarget.value)} />

          <hr className="border-line" />

          <TextInput label="Your email" type="email" required value={email}
            autoComplete="username" data-testid="first-run-email"
            description="What you will sign in with. This account administers the whole deployment."
            onChange={(e) => setEmail(e.currentTarget.value)} />
          <TextInput label="Your name" required={false} value={name} autoComplete="name"
            onChange={(e) => setName(e.currentTarget.value)} />

          <div className="grid gap-4 sm:grid-cols-2">
            <PasswordInput label="Password" required value={password}
              autoComplete="new-password" data-testid="first-run-password"
              onChange={(e) => setPassword(e.currentTarget.value)} />
            <PasswordInput label="Again" required value={again}
              autoComplete="new-password"
              onChange={(e) => setAgain(e.currentTarget.value)} />
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <TextInput label="Organisation" required value={org} placeholder="Acme Logistics"
              disabled={pinned('CRONOS_ORG')} data-testid="first-run-org"
              description={pinned('CRONOS_ORG') ? 'Fixed by CRONOS_ORG.' : undefined}
              onChange={(e) => setOrg(e.currentTarget.value)} />
            <TextInput label="First project" required value={project} placeholder="Finance"
              disabled={pinned('CRONOS_PROJECT')} data-testid="first-run-project"
              description={pinned('CRONOS_PROJECT') ? 'Fixed by CRONOS_PROJECT.' : undefined}
              onChange={(e) => setProject(e.currentTarget.value)} />
          </div>

          <TextInput label="Definition store" required={false} value={storeDsn}
            disabled={pinned('CRONOS_STORE_DSN')} data-testid="first-run-store"
            placeholder="postgres://cronos:…@localhost/cronos?sslmode=disable"
            description={pinned('CRONOS_STORE_DSN')
              ? 'Fixed by CRONOS_STORE_DSN.'
              : 'Where definitions, accounts and run history live. Leave it empty for a file-backed deployment — no sign-in, no history, no sharing.'}
            onChange={(e) => setStoreDsn(e.currentTarget.value)} />

          {fixed.length > 0 && (
            <p className="text-micro text-ink-secondary">
              The environment already sets {fixed.join(', ')}, and the
              environment wins — those fields are fixed here.
            </p>
          )}
        </div>

        {error && (
          <p className="mt-4 text-small text-bad" data-testid="first-run-error">{error}</p>
        )}

        <Button type="submit" fullWidth mt={24} loading={busy} data-testid="first-run-submit">
          Configure and restart
        </Button>
      </form>
    </main>
  )
}

/**
 * The driver a DSN implies.
 *
 * Guessed rather than asked, because somebody who has pasted a postgres:// URL
 * has already answered the question and a second field asking it again is a
 * field they can contradict.
 */
function driverFor(dsn: string): string {
  const s = dsn.trim().toLowerCase()
  if (s.startsWith('postgres://') || s.startsWith('postgresql://')) return 'postgres'
  if (s.startsWith('file:') || s.endsWith('.db') || s.endsWith('.sqlite')) return 'sqlite'
  if (s.startsWith('mysql://') || s.includes('@tcp(')) return 'mysql'
  if (s.startsWith('sqlserver://')) return 'sqlserver'
  return 'postgres'
}
