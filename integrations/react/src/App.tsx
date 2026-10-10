import { useCallback, useEffect, useState } from 'react'
import { CronosReport } from '@cronos/react'

/*
 * Where cronos is. A constant of the build — VITE_CRONOS_URL — and never
 * anything a visitor can set: @cronos/react loads the report element from
 * here, so this decides what code runs on the page.
 */
const CRONOS_URL: string = import.meta.env.VITE_CRONOS_URL ?? 'http://localhost:8787'

interface Session { user: string | null; users: string[] }
interface Issued { token: string; expiresAt: number }

const post = (path: string, body: unknown = {}) =>
  fetch(path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  }).then((res) => res.json())

/*
 * The token, from our own backend, renewed before it runs out.
 *
 * The backend knows who is signed in and signs the scope; this hook only
 * carries the result. Tokens are short-lived on purpose, so a tab left open
 * asks for the next one at four fifths of the way through each.
 */
function useReportToken(user: string | null): string | null {
  const [issued, setIssued] = useState<Issued | null>(null)

  useEffect(() => {
    if (!user) return setIssued(null)
    let live = true
    let timer: ReturnType<typeof setTimeout>

    const renew = async () => {
      const next = (await post('/api/report-token')) as Issued
      if (!live || !next.token) return
      setIssued(next)
      timer = setTimeout(renew, Math.max(30_000, (next.expiresAt - Date.now()) * 0.8))
    }
    void renew()

    return () => { live = false; clearTimeout(timer) }
  }, [user])

  return user ? issued?.token ?? null : null
}

export function App() {
  const [session, setSession] = useState<Session>({ user: null, users: [] })
  const [problem, setProblem] = useState<string | null>(null)
  const token = useReportToken(session.user)

  const refresh = useCallback(async () => {
    setProblem(null)
    setSession(await fetch('/api/session').then((res) => res.json()))
  }, [])
  useEffect(() => { void refresh() }, [refresh])

  return (
    <>
      <header>
        <h1>Acme Billing</h1>
        <nav data-testid="account">
          {session.user ? (
            <>
              <span data-testid="user">{session.user}</span>
              <button data-testid="sign-out" onClick={() => post('/api/sign-out').then(refresh)}>
                Sign out
              </button>
            </>
          ) : session.users.map((name) => (
            <button key={name} data-testid={`sign-in-${name}`}
              onClick={() => post('/api/sign-in', { user: name }).then(refresh)}>
              Sign in as {name}
            </button>
          ))}
        </nav>
      </header>

      <main data-testid="stage">
        {!session.user && <p className="notice" data-testid="notice">Sign in to see your billing summary.</p>}
        {problem && <p className="notice" data-testid="notice">{problem}</p>}
        {token && (
          <CronosReport
            endpoint={CRONOS_URL}
            token={token}
            report="billing-summary"
            /* The message is written for a person: an expired link, a report
               that is gone, a server that could not be reached. */
            onError={(e) => setProblem(e.message)}
            onLoad={() => setProblem(null)}
          />
        )}
      </main>
    </>
  )
}
