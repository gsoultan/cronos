import { Link, useRouterState } from '@tanstack/react-router'
import { Icon, type IconName } from './Icon'

/* `also` is the other paths a section owns, so its entry stays lit there. */
const NAV: { to: string; label: string; icon: IconName; also?: string[] }[] = [
  { to: '/', label: 'Reports', icon: 'reports', also: ['/reports'] },
  { to: '/data', label: 'Data', icon: 'data' },
  { to: '/schedules', label: 'Schedules', icon: 'schedules', also: ['/activity'] },
  { to: '/settings', label: 'Settings', icon: 'settings' },
]

/**
 * The four destinations.
 *
 * None of them Dashboards: there is no Dashboard artifact. A dashboard is a
 * report whose only output is interactive — see docs/report-format.md.
 *
 * Run history is the History tab of Schedules, at its own URL. It still lists
 * every run across every schedule — "did anything fail last night" is not a
 * question about one schedule — but it is where people look after asking
 * what is scheduled, so it no longer needs a place of its own in the rail.
 *
 * One line per entry. Each used to carry a second line describing itself,
 * which is read once and then is ten rows of text between somebody and the
 * word they are looking for.
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
          const active = (item.to === '/' ? path === '/' : path.startsWith(item.to))
            || (item.also ?? []).some((p) => path.startsWith(p))

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
                <span className={`min-w-0 text-body font-medium ${collapsed ? 'md:hidden' : ''}`}>
                  {item.label}
                </span>
              </Link>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
