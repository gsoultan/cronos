/*
 * A client-side React application, which is what a cronos customer has.
 *
 * No framework, no server rendering, no hydration — Vite + React + a router
 * they wrote themselves is the shape of the thing embedding us. The harness
 * imports the *built* package rather than src, so the check exercises what
 * actually ships, externals and all.
 */
import { StrictMode, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { CronosReport } from '@cronos/react'

const base = location.origin

function App() {
  const [status, setStatus] = useState<string | null>(null)
  const [mounted, setMounted] = useState(true)
  const [events, setEvents] = useState<string[]>([])
  const [renders, setRenders] = useState(0)

  return (
    <main>
      <button data-testid="filter" onClick={() => setStatus('overdue')}>Overdue only</button>
      <button data-testid="clear" onClick={() => setStatus(null)}>Clear</button>
      <button data-testid="unmount" onClick={() => setMounted(false)}>Unmount</button>
      {/* Re-renders the parent without touching any prop the report cares
          about — the case that must NOT refetch. */}
      <button data-testid="rerender" onClick={() => setRenders((n) => n + 1)}>
        Re-render {renders}
      </button>
      <p data-testid="events">{events.join(',')}</p>

      {mounted && (
        <CronosReport
          endpoint={base}
          token="tok"
          report="monthly"
          /* Deliberately an inline object: a new identity on every render,
             which is how everyone writes it. */
          filters={status ? { status: { op: 'eq', values: [status] } } : {}}
          onLoad={() => setEvents((e) => [...e, 'load'])}
          onError={() => setEvents((e) => [...e, 'error'])}
        />
      )}
    </main>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode><App /></StrictMode>,
)
