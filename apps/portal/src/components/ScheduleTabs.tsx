import { Link, useRouterState } from '@tanstack/react-router'

const TABS = [
  { to: '/schedules', label: 'All schedules', testid: 'tab-schedules' },
  { to: '/activity', label: 'History', testid: 'tab-history' },
] as const

/**
 * What is set to go out, and what went out.
 *
 * Two routes behind one entry in the rail. Links rather than buttons, so each
 * tab keeps the address it always had and a bookmark to /activity still lands
 * on the history.
 */
export function ScheduleTabs() {
  const path = useRouterState({ select: (s) => s.location.pathname })
  return (
    <nav aria-label="Schedules" className="mb-4 flex gap-1 overflow-x-auto border-b border-line">
      {TABS.map((t) => {
        const on = path.startsWith(t.to)
        return (
          <Link key={t.to} to={t.to} data-testid={t.testid} aria-current={on ? 'page' : undefined}
            className={`shrink-0 border-b-2 px-3 py-2.5 text-small font-medium no-underline ${
              on ? 'border-accent text-ink' : 'border-transparent text-ink-secondary hover:text-ink'}`}>
            {t.label}
          </Link>
        )
      })}
    </nav>
  )
}
