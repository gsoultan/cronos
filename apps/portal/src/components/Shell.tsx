import { useEffect, useState } from 'react'
import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import { Header } from './Header'
import { Icon, type IconName } from './Icon'
import { WorkspaceSwitcher } from './WorkspaceSwitcher'
import { useWorkspace } from '../lib/WorkspaceContext'
import { useSidebar } from '../lib/useSidebar'

const NAV: { to: string; label: string; hint: string; icon: IconName }[] = [
  { to: '/', label: 'Reports', hint: 'Run and schedule reports', icon: 'reports' },
  { to: '/dashboards', label: 'Dashboards', hint: 'Live views for your team', icon: 'dashboards' },
  { to: '/data', label: 'Data', hint: 'Sources and datasets', icon: 'data' },
  { to: '/schedules', label: 'Schedules', hint: 'What sends, and when', icon: 'schedules' },
  { to: '/settings', label: 'Settings', hint: 'People and projects', icon: 'settings' },
]

/**
 * Header across the top, collapsible rail beneath it, content beside.
 *
 * Collapsed, the rail keeps its icons and gains tooltips rather than
 * disappearing — an icon-only rail is still navigable, a hidden one is a
 * memory test.
 */
export function Shell() {
  const path = useRouterState({ select: (s) => s.location.pathname })
  const { org, project, setContext } = useWorkspace()
  const { collapsed, toggle } = useSidebar()
  const [theme, setTheme] = useState<'light' | 'dark'>('light')

  /* Mantine keys its own dark steps off `data-mantine-color-scheme`; our tokens
     key off `data-theme`. Both are set from one state so they cannot disagree. */
  useEffect(() => {
    const root = document.documentElement
    root.dataset.theme = theme
    root.dataset.mantineColorScheme = theme
  }, [theme])

  return (
    <div className="min-h-full">
      <a href="#main" className="skip-link z-100 bg-surface px-4 py-2">Skip to content</a>

      <Header collapsed={collapsed} onToggleSidebar={toggle} theme={theme}
        onToggleTheme={() => setTheme((t) => (t === 'light' ? 'dark' : 'light'))} />

      <div className={`grid ${collapsed ? 'md:grid-cols-[64px_1fr]' : 'md:grid-cols-[248px_1fr]'}`}>
        <aside data-testid="sidebar" data-collapsed={collapsed}
          className={`flex flex-col gap-4 border-line bg-surface p-3 max-md:border-b
                      md:sticky md:top-14 md:h-[calc(100vh-3.5rem)] md:border-r
                      ${collapsed ? 'md:items-center' : ''}`}>
          <WorkspaceSwitcher org={org} project={project} onChange={setContext}
            collapsed={collapsed} />

          <nav aria-label="Main">
            <ul className="grid gap-1 max-md:auto-cols-max max-md:grid-flow-col max-md:overflow-x-auto">
              {NAV.map((item) => {
                /* Reports owns '/' and every '/reports/*' route, so opening or
                   building a report keeps its own section lit. */
                const active = item.to === '/'
                  ? path === '/' || path.startsWith('/reports')
                  : path.startsWith(item.to)
                /* Native `title`, not a Mantine Tooltip: this is the app shell,
                   so anything imported here lands in the eager bundle — the
                   library tooltip alone costs ~30 KB there, and a nav rail hint
                   is precisely what the attribute is for. */
                const link = (
                  <Link to={item.to} aria-current={active ? 'page' : undefined}
                    aria-label={collapsed ? item.label : undefined}
                    title={collapsed ? item.label : undefined}
                    className={`flex items-center gap-3 rounded-md no-underline transition-colors
                      duration-150 ease-out-quick hover:bg-hover
                      ${collapsed ? 'size-10 justify-center' : 'px-3 py-2'}
                      ${active ? 'bg-accent-wash text-ink' : 'text-ink'}`}>
                    <Icon name={item.icon} className={`size-[18px] ${
                      active ? 'text-accent' : 'text-ink-muted'}`} />
                    {!collapsed && (
                      <span className="min-w-0">
                        <span className="block text-body font-medium">{item.label}</span>
                        <span className={`block text-caption max-md:hidden ${
                          active ? 'text-ink-secondary' : 'text-ink-muted'}`}>
                          {item.hint}
                        </span>
                      </span>
                    )}
                  </Link>
                )
                return <li key={item.to}>{link}</li>
              })}
            </ul>
          </nav>
        </aside>

        <main id="main" className="w-full max-w-[1600px] p-6 md:px-8 md:py-8">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
