import { useEffect, useRef, useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { Icon } from './Icon'
import { Brand } from './Brand'
import { useWorkspace } from '../lib/WorkspaceContext'

interface Props {
  collapsed: boolean
  onToggleSidebar: () => void
  theme: 'light' | 'dark'
  onToggleTheme: () => void
}

const TITLES: Record<string, string> = {
  '/': 'Reports',
  '/data': 'Data',
  '/schedules': 'Schedules',
  '/settings': 'Settings',
  '/account': 'Your account',
  '/reports/new': 'New report',
}

/**
 * The top bar: brand, where you are, and the controls that belong to the whole
 * app rather than to the page.
 *
 * It spans the full width above the sidebar rather than sitting beside it, so
 * the brand and the collapse control stay put when the rail narrows — a
 * collapse toggle that moves as you collapse is a small cruelty.
 */
export function Header({ collapsed, onToggleSidebar, theme, onToggleTheme }: Props) {
  const path = useRouterState({ select: (s) => s.location.pathname })
  const { org, project } = useWorkspace()

  const title = TITLES[path] ?? (path.startsWith('/reports/') ? 'Report' : '')

  return (
    <header className="sticky top-0 z-50 flex h-14 items-center gap-3 border-b border-line
                       bg-surface px-3">
      <button type="button" onClick={onToggleSidebar}
        data-testid="sidebar-toggle"
        aria-label={`${collapsed ? 'Expand' : 'Collapse'} sidebar`}
        aria-expanded={!collapsed}
        title={`${collapsed ? 'Expand' : 'Collapse'} sidebar  [`}
        className="grid size-8 cursor-pointer place-items-center rounded-md text-ink-secondary
                   hover:bg-hover hover:text-ink">
        <Icon name="sidebar" />
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
        {title && (
          <>
            <span className="text-line" aria-hidden>/</span>
            <span className="truncate text-small font-medium text-ink">{title}</span>
          </>
        )}
      </nav>

      <div className="ml-auto flex items-center gap-1">
        <GlobalSearch />

        <button type="button" onClick={onToggleTheme}
          aria-label={`Switch to ${theme === 'light' ? 'dark' : 'light'} theme`}
          className="grid size-8 cursor-pointer place-items-center rounded-md text-ink-secondary
                     hover:bg-hover hover:text-ink">
          <Icon name={theme === 'light' ? 'moon' : 'sun'} />
        </button>

        <Link to="/account" aria-label="Your account" title="Your account"
          data-testid="account-link"
          className="grid size-8 place-items-center rounded-full border border-line
                     bg-sunken text-small font-semibold text-ink-secondary no-underline
                     hover:border-accent">
          DR
        </Link>
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
