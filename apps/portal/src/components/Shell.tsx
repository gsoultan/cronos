import { useEffect, useState } from 'react'
import { Outlet, useRouterState } from '@tanstack/react-router'
import { Header } from './Header'
import { SampleBanner } from './SampleBanner'
import { NavRail } from './NavRail'
import { WorkspaceSwitcher } from './WorkspaceSwitcher'
import { useWorkspace } from '../lib/WorkspaceContext'
import { useSidebar } from '../lib/useSidebar'

/**
 * Header across the top, navigation beneath it, content beside — or in front of
 * it, on a phone.
 *
 * The rail is two different things at two sizes and pretending otherwise is
 * what makes responsive shells bad. On a desktop it is a persistent column you
 * narrow to buy width. On a 390px screen a persistent column is 45% of the
 * viewport spent before the first word of content, so there it is an overlay
 * you open, and it closes itself the moment you have chosen a destination.
 */
export function Shell() {
  const path = useRouterState({ select: (s) => s.location.pathname })
  const { org, project, setContext, branding } = useWorkspace()
  const { collapsed, toggle } = useSidebar()
  const [drawer, setDrawer] = useState(false)
  const [theme, setTheme] = useState<'light' | 'dark'>('light')

  /* Mantine keys its own dark steps off `data-mantine-color-scheme`; our tokens
     key off `data-theme`. Both are set from one state so they cannot disagree. */
  useEffect(() => {
    const root = document.documentElement
    root.dataset.theme = theme
    root.dataset.mantineColorScheme = theme
  }, [theme])

  /* Navigating closes the drawer. It sits on top of the page it just took you
     to, so leaving it open hides the result of the tap that opened it. */
  useEffect(() => { setDrawer(false) }, [path])

  useEffect(() => {
    if (!drawer) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setDrawer(false) }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [drawer])

  return (
    <div className="min-h-full">
      <a href="#main" className="skip-link z-100 bg-surface px-4 py-2">Skip to content</a>

      <Header collapsed={collapsed} onToggleSidebar={toggle}
        drawerOpen={drawer} onToggleDrawer={() => setDrawer((d) => !d)} theme={theme}
        onToggleTheme={() => setTheme((t) => (t === 'light' ? 'dark' : 'light'))} />

      <SampleBanner />

      {/* minmax(0,…) on every column. A bare `1fr` is `minmax(auto,1fr)`, which
          refuses to shrink below its content — so a wide child pushed the whole
          grid past the viewport and the page scrolled sideways on mobile. */}
      <div className={`grid grid-cols-[minmax(0,1fr)] ${
        collapsed ? 'md:grid-cols-[64px_minmax(0,1fr)]' : 'md:grid-cols-[248px_minmax(0,1fr)]'}`}>
        {/* Closed, the drawer is `invisible` and not merely translated away:
            an offscreen rail still takes tab stops and still reads to a screen
            reader, so it becomes four links that focus nothing you can see. */}
        <aside id="nav-rail" data-testid="sidebar" data-collapsed={collapsed}
          data-drawer={drawer ? 'open' : 'closed'}
          className={`flex min-w-0 flex-col gap-3 border-line bg-surface p-3
                      max-md:fixed max-md:inset-y-0 max-md:top-14 max-md:left-0 max-md:z-40
                      max-md:w-[min(300px,82vw)] max-md:overflow-y-auto max-md:border-r
                      max-md:shadow-pop max-md:transition-transform max-md:duration-200
                      md:sticky md:top-14 md:h-[calc(100vh-3.5rem)] md:gap-4 md:border-r
                      ${drawer ? 'max-md:translate-x-0' : 'max-md:invisible max-md:-translate-x-full'}
                      ${collapsed ? 'md:items-center' : ''}`}>
          <WorkspaceSwitcher org={org} project={project} onChange={setContext}
            collapsed={collapsed && !drawer} mark={branding.mark?.url} />
          <NavRail collapsed={collapsed && !drawer} />
        </aside>

        {drawer && (
          <button type="button" aria-label="Close navigation" data-testid="drawer-scrim"
            onClick={() => setDrawer(false)}
            className="fixed inset-0 top-14 z-30 bg-ink/25 md:hidden" />
        )}

        <main id="main" className="w-full min-w-0 max-w-[1600px] p-4 md:px-8 md:py-8">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
