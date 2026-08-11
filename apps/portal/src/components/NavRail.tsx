import { Link, useRouterState } from '@tanstack/react-router'
import { Icon, type IconName } from './Icon'

const NAV: { to: string; label: string; hint: string; icon: IconName }[] = [
  { to: '/', label: 'Reports', hint: 'Run and schedule reports', icon: 'reports' },
  { to: '/data', label: 'Data', hint: 'Sources and datasets', icon: 'data' },
  { to: '/schedules', label: 'Schedules', hint: 'What sends, and when', icon: 'schedules' },
  { to: '/activity', label: 'Activity', hint: 'What ran, and who got it', icon: 'activity' },
  { to: '/settings', label: 'Settings', hint: 'People and projects', icon: 'settings' },
]

/**
 * The five destinations.
 *
 * Five, and none of them Dashboards: there is no Dashboard artifact. A
 * dashboard is a report whose only output is interactive — see
 * docs/report-format.md.
 *
 * Activity sits beside Schedules rather than inside them because a run is not
 * a property of the schedule that caused it. Somebody asking "did anything
 * fail last night" is not asking about one schedule, and making them open each
 * in turn to find out is the question restated as a chore.
 *
 * `collapsed` narrows the rail at md and up only. Below that the rail is a
 * drawer, and a drawer that opens to four unlabelled icons is a worse memory
 * test than the one collapsing was meant to avoid — so the collapse styling is
 * all `md:`-prefixed and the labels survive on a phone either way.
 */
export function NavRail({ collapsed }: { collapsed: boolean }) {
  const path = useRouterState({ select: (s) => s.location.pathname })

  return (
    <nav aria-label="Main" className="min-w-0">
      <ul className="grid gap-1">
        {NAV.map((item) => {
          /* Reports owns '/' and every '/reports/*' route, so opening or
             building a report keeps its own section lit. */
          const active = item.to === '/'
            ? path === '/' || path.startsWith('/reports')
            : path.startsWith(item.to)

          return (
            <li key={item.to}>
              {/* Native `title`, not a Mantine Tooltip: this is the app shell,
                  so anything imported here lands in the eager bundle — the
                  library tooltip alone costs ~30 KB there, and a nav rail hint
                  is precisely what the attribute is for. */}
              <Link to={item.to} aria-current={active ? 'page' : undefined}
                title={collapsed ? item.label : undefined}
                className={`flex shrink-0 items-center gap-3 rounded-md px-3 py-2 no-underline
                  transition-colors duration-150 ease-out-quick hover:bg-hover
                  ${collapsed ? 'md:size-10 md:justify-center md:p-0' : ''}
                  ${active ? 'bg-accent-wash text-ink' : 'text-ink'}`}>
                <Icon name={item.icon} className={`size-[18px] shrink-0 ${
                  active ? 'text-accent' : 'text-ink-muted'}`} />
                <span className={`min-w-0 ${collapsed ? 'md:hidden' : ''}`}>
                  <span className="block text-body font-medium">{item.label}</span>
                  <span className={`block text-caption ${
                    active ? 'text-ink-secondary' : 'text-ink-muted'}`}>
                    {item.hint}
                  </span>
                </span>
              </Link>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
