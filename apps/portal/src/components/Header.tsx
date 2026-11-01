import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { Icon } from './Icon'
import { Brand } from './Brand'
import { currentUser, endSession, type SignedInUser } from '../lib/api'

/* The list behind ⌘K, fetched the first time somebody opens it. */
const JumpTo = lazy(() => import('./JumpTo').then((m) => ({ default: m.JumpTo })))

interface Props {
  collapsed: boolean
  onToggleSidebar: () => void
  /** Mobile only: the rail is an overlay below md, not a column. */
  drawerOpen: boolean
  onToggleDrawer: () => void
  theme: 'light' | 'dark'
  onToggleTheme: () => void
}

const TOGGLE = `grid size-8 cursor-pointer place-items-center rounded-md text-ink-secondary
                hover:bg-hover hover:text-ink`

/*
 * What the breadcrumb calls each page.
 *
 * One entry per top-level route. Activity was missing, so its breadcrumb read
 * "acme / finance" and stopped — visible in a screenshot beside five pages that
 * name themselves, and invisible in sample mode for the same reason everything
 * else was.
 */
const SECTIONS: Record<string, string> = {
  '': 'Reports',
  reports: 'Reports',
  data: 'Data',
  schedules: 'Schedules',
  // Run history is a tab of Schedules, so it is named as one.
  activity: 'Schedules',
  settings: 'Settings',
  account: 'Your account',
}

/* What /data holds, since it holds two things and they are not interchangeable. */
const KINDS: Record<string, string> = { sources: 'Sources', datasets: 'Datasets' }

/*
 * The trail to a page, from its path.
 *
 * A map of fixed titles could only ever name the pages with no argument in
 * them, and everything below a section has one: the editors read "acme /
 * finance" and stopped, which is the same gap Activity had. The old fallback
 * said "Report" over a page editing a specific report — a label that is true of
 * every one of them and identifies none.
 *
 * The name from the path rather than the loaded document's title, deliberately.
 * The breadcrumb is drawn before the fetch resolves, and a crumb that appears
 * blank and then fills in is a header that moves while somebody is reading it.
 * The identifier is also what they typed to get here.
 *
 * "Edit" is not a crumb. The editor is unmistakably an editor, and the last
 * crumb should name the thing rather than the verb.
 */
export function crumbs(path: string): string[] {
  const [section, ...rest] = path.replace(/^\/+|\/+$/g, '').split('/')
  const head = SECTIONS[section ?? '']
  if (!head) return []

  const parts = rest.filter((p) => p !== 'edit')
  // The one page below a section with no name of its own, because it is the
  // page where somebody chooses one.
  if (section === 'reports' && parts[0] === 'new') return [head, 'New report']

  if (section === 'activity') return [head, 'History']

  const kind = KINDS[parts[0] ?? '']
  return [head, ...(kind ? [kind, ...parts.slice(1)] : parts)]
}

/**
 * The top bar: brand, where you are, and the controls that belong to the whole
 * app rather than to the page.
 *
 * It spans the full width above the sidebar rather than sitting beside it, so
 * the brand and the collapse control stay put when the rail narrows — a
 * collapse toggle that moves as you collapse is a small cruelty.
 */
export function Header({ collapsed, onToggleSidebar, drawerOpen, onToggleDrawer,
  theme, onToggleTheme }: Props) {
  const path = useRouterState({ select: (s) => s.location.pathname })

  const trail = crumbs(path)

  return (
    <header className="sticky top-0 z-50 flex h-14 items-center gap-3 border-b border-line
                       bg-surface px-3">
      {/* Two controls, not one with a responsive label. Below md the rail is an
          overlay you open; at md and up it is a column you narrow. Those are
          different promises, and an accessible name cannot branch on a media
          query — so each size gets the button whose name is true. */}
      <button type="button" onClick={onToggleSidebar}
        data-testid="sidebar-toggle"
        aria-label={`${collapsed ? 'Expand' : 'Collapse'} sidebar`}
        aria-expanded={!collapsed}
        title={`${collapsed ? 'Expand' : 'Collapse'} sidebar  [`}
        className={`${TOGGLE} max-md:hidden`}>
        <Icon name="sidebar" />
      </button>
      <button type="button" onClick={onToggleDrawer}
        data-testid="drawer-toggle"
        aria-label={`${drawerOpen ? 'Close' : 'Open'} navigation`}
        aria-expanded={drawerOpen} aria-controls="nav-rail"
        className={`${TOGGLE} md:hidden`}>
        {/* A hamburger, not the panel-split glyph the desktop control uses.
            Same job, but on a phone that glyph reads as "split the view" and
            this one is the one symbol everybody already knows. */}
        <Icon name="menu" />
      </button>

      {/* The product's own identity. Organisation branding lives in the
          workspace switcher, not here: cronos is multi-organisation, and a
          header that changed identity on switch would confuse which product
          you are in. In an embedded view the customer's wordmark replaces
          this entirely — that is white-label, and it is licensed. */}
      <Link to="/" aria-label="cronos home" className="no-underline">
        <Brand />
      </Link>

      {/* Where you are within the workspace. Which workspace is the switcher's
          job, directly below; naming the organisation and project here too
          said the same thing twice on every page. */}
      <nav aria-label="Breadcrumb" className="ml-2 hidden min-w-0 items-center gap-2 md:flex">
        {trail.map((crumb, i) => (
          <span key={crumb} className="flex min-w-0 items-center gap-2">
            <span className="text-line" aria-hidden>/</span>
            {/* Only the last crumb is where you are; the ones before it are
                the way you got here, and weighting them all equally makes the
                header read as a sentence nobody wrote. */}
            <span className={`truncate text-small ${i === trail.length - 1
              ? 'font-medium text-ink' : 'text-ink-secondary'}`}>
              {crumb}
            </span>
          </span>
        ))}
      </nav>

      <div className="ml-auto flex items-center gap-2">
        <JumpButton />
        <AccountMenu theme={theme} onToggleTheme={onToggleTheme} />
      </div>
    </header>
  )
}

/**
 * Opens ⌘K. A button that looks like a search field, because that is where
 * people look for one; it holds no text of its own, so nothing typed into it
 * can be lost.
 */
function JumpButton() {
  const [open, setOpen] = useState(false)

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setOpen((o) => !o)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <>
      <button type="button" onClick={() => setOpen(true)} data-testid="jump-button"
        aria-label="Jump to a report, dataset, schedule or page" aria-haspopup="dialog"
        className="flex h-8 cursor-pointer items-center gap-2 rounded-md border border-line
                   bg-sunken px-2.5 text-small text-ink-muted hover:border-accent
                   max-sm:w-8 max-sm:justify-center max-sm:px-0">
        <Icon name="search" className="size-4" />
        <span className="max-sm:hidden">Jump to…</span>
        <kbd className="rounded-sm border border-line px-1 text-micro max-sm:hidden">⌘K</kbd>
      </button>
      {open && (
        <Suspense fallback={null}>
          <JumpTo onClose={() => setOpen(false)} />
        </Suspense>
      )}
    </>
  )
}

/**
 * Who is signed in, and the things that belong to them rather than to a page:
 * their account, how the portal looks to them, and leaving.
 *
 * One control instead of three icons in a row. Hand-built rather than
 * Mantine's Menu, because the header is in the eager bundle.
 */
function AccountMenu({ theme, onToggleTheme }: { theme: 'light' | 'dark'; onToggleTheme: () => void }) {
  // Read once per render rather than held in state: a sign-out takes the whole
  // shell down to the sign-in page, so there is nothing here to keep in step.
  const me = currentUser()
  const path = useRouterState({ select: (s) => s.location.pathname })
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLDivElement>(null)

  useEffect(() => { setOpen(false) }, [path])
  useEffect(() => {
    if (!open) return
    const away = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false)
    }
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false) }
    document.addEventListener('mousedown', away)
    window.addEventListener('keydown', esc)
    return () => {
      document.removeEventListener('mousedown', away)
      window.removeEventListener('keydown', esc)
    }
  }, [open])

  const ITEM = `flex w-full cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-left
                text-body text-ink no-underline hover:bg-hover`

  return (
    <div ref={box} className="relative">
      <button type="button" onClick={() => setOpen((o) => !o)} data-testid="account-menu"
        aria-label="Your account" aria-haspopup="menu" aria-expanded={open}
        title={me ? me.email : 'Your account'}
        className="grid size-8 cursor-pointer place-items-center rounded-full border border-line
                   bg-sunken text-small font-semibold text-ink-secondary hover:border-accent">
        {initials(me)}
      </button>
      {open && (
        <div role="menu" aria-label="Your account"
          className="absolute top-10 right-0 z-60 grid w-60 gap-0.5 rounded-lg border border-line
                     bg-surface p-1.5 shadow-pop">
          {me && (
            <div className="border-b border-line px-2.5 pt-1 pb-2">
              <p className="truncate text-body font-medium text-ink">{me.name || me.email}</p>
              {me.name && <p className="truncate text-caption text-ink-muted">{me.email}</p>}
            </div>
          )}
          <Link to="/account" role="menuitem" data-testid="account-link" className={ITEM}>
            <Icon name="user" className="size-4 text-ink-muted" />Your account
          </Link>
          <button type="button" role="menuitem" onClick={onToggleTheme} className={ITEM}
            aria-label={`Switch to ${theme === 'light' ? 'dark' : 'light'} theme`}>
            <Icon name={theme === 'light' ? 'moon' : 'sun'} className="size-4 text-ink-muted" />
            {theme === 'light' ? 'Dark theme' : 'Light theme'}
          </button>
          {me && (
            <button type="button" role="menuitem" onClick={() => void endSession()}
              data-testid="sign-out" className={`${ITEM} border-t border-line`}>
              <Icon name="sign-out" className="size-4 text-ink-muted" />Sign out
            </button>
          )}
        </div>
      )}
    </div>
  )
}

/*
 * Whose session this is, in two letters.
 *
 * Next to a sign-out button, showing the wrong person is worse than showing
 * nobody — this used to be hardcoded initials, which on a shared machine reads
 * as being signed in as somebody else.
 *
 * From the name where there is one, and from the email otherwise, because a
 * directory that gave us an address and no name is common and "?" is not an
 * improvement on the first letter of it.
 */
function initials(me: SignedInUser | null): string {
  if (!me) return '\u00B7'

  const words = (me.name ?? '').trim().split(/\s+/).filter(Boolean)
  const first = words.at(0)
  const last = words.at(-1)
  if (first && last && words.length >= 2) {
    return (first.slice(0, 1) + last.slice(0, 1)).toUpperCase()
  }
  if (first) return first.slice(0, 2).toUpperCase()
  return (me.email || '?').slice(0, 2).toUpperCase()
}
