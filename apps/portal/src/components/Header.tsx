import { useEffect, useRef, useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { Icon } from './Icon'
import { Brand } from './Brand'
import { useWorkspace } from '../lib/WorkspaceContext'
import { currentUser, endSession, type SignedInUser } from '../lib/api'

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
  activity: 'Activity',
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
  const { org, project } = useWorkspace()
  // Read once per render rather than held in state: a sign-out takes the whole
  // shell down to the sign-in page, so there is nothing here to keep in step.
  const me = currentUser()

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

      {/* Where you are. The switcher in the rail is how you change it; this is
          only ever a label, so it is not a second control competing with it. */}
      <nav aria-label="Breadcrumb" className="ml-2 hidden min-w-0 items-center gap-2 md:flex">
        <span className="text-line" aria-hidden>/</span>
        <span className="truncate text-small text-ink-secondary">{org.name}</span>
        <span className="text-line" aria-hidden>/</span>
        <span className="truncate text-small text-ink-secondary">{project.name}</span>
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

      <div className="ml-auto flex items-center gap-1">
        <GlobalSearch />

        <button type="button" onClick={onToggleTheme}
          aria-label={`Switch to ${theme === 'light' ? 'dark' : 'light'} theme`}
          className="grid size-8 cursor-pointer place-items-center rounded-md text-ink-secondary
                     hover:bg-hover hover:text-ink">
          <Icon name={theme === 'light' ? 'moon' : 'sun'} />
        </button>

        <Link to="/account" aria-label="Your account" title={me ? me.email : 'Your account'}
          data-testid="account-link"
          className="grid size-8 place-items-center rounded-full border border-line
                     bg-sunken text-small font-semibold text-ink-secondary no-underline
                     hover:border-accent">
          {initials(me)}
        </Link>

        {me && (
          <button type="button" onClick={() => void endSession()}
            aria-label="Sign out" title="Sign out" data-testid="sign-out"
            className="grid size-8 cursor-pointer place-items-center rounded-md
                       text-ink-secondary hover:bg-hover hover:text-ink">
            <Icon name="sign-out" />
          </button>
        )}
      </div>
    </header>
  )
}

/** Collapses to an icon on narrow screens; ⌘K focuses it anywhere. */
function GlobalSearch() {
  const input = useRef<HTMLInputElement>(null)
  const [value, setValue] = useState('')

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        input.current?.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <div className="relative hidden sm:block">
      <span className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2
                       text-ink-muted">
        <Icon name="search" className="size-4" />
      </span>
      <input ref={input} type="search" value={value} aria-label="Search everything"
        onChange={(e) => setValue(e.currentTarget.value)}
        placeholder="Search reports and data…"
        className="h-8 w-56 rounded-md border border-line bg-sunken pr-10 pl-8 text-small
                   text-ink placeholder:text-ink-muted focus:w-72 focus:border-accent
                   focus:bg-surface" />
      <kbd className="pointer-events-none absolute top-1/2 right-2 -translate-y-1/2
                      rounded-sm border border-line px-1 text-micro text-ink-muted">
        ⌘K
      </kbd>
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
