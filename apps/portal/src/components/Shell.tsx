import { lazy, Suspense, useEffect, useState } from 'react'
import { Outlet, useRouterState } from '@tanstack/react-router'
import { Header } from './Header'
import { SampleBanner } from './SampleBanner'
import {
  adoptSessionFromFragment, mustEnrol, needsSignIn, setupNeeded, SIGNED_OUT,
} from '../lib/api'

/* Lazy, because the sign-in page pulls Mantine's password field and most loads
   never show it — a signed-in author, and every load in sample mode. It was
   ten kilobytes in the eager bundle for a page shown once a day. */
const SignInPage = lazy(() =>
  import('../routes/SignInPage').then((m) => ({ default: m.SignInPage })))

const SetupPage = lazy(() =>
  import('../routes/SetupPage').then((m) => ({ default: m.SetupPage })))

const MustEnrolPage = lazy(() =>
  import('../routes/MustEnrolPage').then((m) => ({ default: m.MustEnrolPage })))
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
  /* Bumped after signing in, so the shell re-reads the session. A router
     redirect would be tidier and would also mean the sign-in page has a URL
     somebody can be sent to while their session is fine. */
  const [session, setSession] = useState(0)

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

  /* A session can end while somebody is looking at a page — a token expires,
     or the server restarts with a new signing key. The request that discovers
     it clears the session and says so; this is what listens. */
  useEffect(() => {
    const ended = () => setSession((n) => n + 1)
    globalThis.addEventListener(SIGNED_OUT, ended)
    return () => globalThis.removeEventListener(SIGNED_OUT, ended)
  }, [])

  useEffect(() => {
    if (!drawer) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setDrawer(false) }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [drawer])

  /* A session handed back by an identity provider arrives in the URL
     fragment. Taken before the first render decides whether anybody is signed
     in, and removed from the address bar in the same breath. */
  const [adopted] = useState(() => adoptSessionFromFragment())
  void adopted

  /* Undefined until the server has answered. */
  const [setupWanted, setSetupWanted] = useState<boolean | undefined>(undefined)
  useEffect(() => {
    if (!needsSignIn()) {
      setSetupWanted(false)
      return
    }
    let live = true
    void setupNeeded().then((yes) => { if (live) setSetupWanted(yes) })
    return () => { live = false }
  }, [session])

  /* Two pages stand on their own, before the sign-in check rather than after.
     A shared report, because whoever follows the link has no account here and
     asking them to sign in to read something they were deliberately given
     without one would be the interface undoing the feature. And an invitation,
     because the person opening it does not have an account *yet* — sending
     them to a sign-in page they cannot pass is the loop this feature exists to
     break. */
  if (path.startsWith('/s/') || path === '/invitation') {
    return (
      <Suspense fallback={<main className="min-h-screen bg-canvas" />}>
        <Outlet />
      </Suspense>
    )
  }

  /* Signed in, and to one thing only: this project requires a second factor
     and this account has none. The wizard and nothing else, because the shell
     around it would be a navigation bar over panels that each answer 403 — and
     the server refuses every one of those routes anyway. */
  if (mustEnrol()) {
    return (
      <Suspense fallback={<main className="min-h-screen bg-canvas" />}>
        <MustEnrolPage onDone={() => setSession((n) => n + 1)} />
      </Suspense>
    )
  }

  /* A configured server with nobody signed in is the sign-in page and nothing
     else — no shell, no navigation to pages that would only 401. Sample mode
     never reaches here, which is what keeps the interface workable before a
     server exists. */
  if (needsSignIn()) {
    /* A deployment with no accounts at all shows setup instead of a sign-in
       form nobody can pass. Asked of the server rather than guessed, and the
       answer is only ever "yes" while it is true — the endpoint closes itself
       the moment an account exists. Undecided means the question is still in
       flight, and neither page is shown, because flashing a sign-in form and
       replacing it is worse than a blank moment. */
    if (setupWanted === undefined) {
      return <main className="min-h-screen bg-canvas" />
    }
    return (
      <Suspense fallback={<main className="min-h-screen bg-canvas" />}>
        {setupWanted
          ? <SetupPage onDone={() => { setSetupWanted(false); setSession((n) => n + 1) }} />
          : <SignInPage onSignedIn={() => setSession((n) => n + 1)} />}
      </Suspense>
    )
  }

  return (
    <div className="min-h-full" key={session}>
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
