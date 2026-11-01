import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Icon } from './Icon'
import { useCatalog } from '../lib/useCatalog'
import { useWorkspace } from '../lib/WorkspaceContext'
import { reportsIn } from '../lib/workspace'
import { reports } from '../lib/mock'
import { catalogEntries, matchEntries, PAGES, type JumpEntry } from '../lib/jump'

/**
 * ⌘K: type a few letters, press Enter, be there.
 *
 * The header used to carry a search box that kept what was typed into it and
 * did nothing else. This is what it promised. Loaded on first open rather than
 * with the shell, so the eager bundle carries the button and not the list.
 *
 * The same catalogue query every list page already makes, so opening it on a
 * page that has loaded one costs no request.
 */
export function JumpTo({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const catalog = useCatalog()
  const { project } = useWorkspace()
  const [typed, setTyped] = useState('')
  const [at, setAt] = useState(0)
  const input = useRef<HTMLInputElement>(null)

  const entries = useMemo<JumpEntry[]>(() => {
    if (catalog.live) return [...PAGES, ...(catalog.data ? catalogEntries(catalog.data) : [])]
    // Sample mode has no catalogue; the fixture reports are what its pages show.
    return [...PAGES, ...reportsIn(reports, project).map((r): JumpEntry => ({
      key: `report:${r.name}`, label: r.label, kind: 'Report',
      to: '/reports/$name', params: { name: r.name },
    }))]
  }, [catalog.live, catalog.data, project])

  const found = useMemo(() => matchEntries(entries, typed), [entries, typed])

  useEffect(() => { input.current?.focus() }, [])
  useEffect(() => { setAt(0) }, [typed])

  function go(e: JumpEntry | undefined) {
    if (!e) return
    onClose()
    void navigate({ to: e.to, params: e.params })
  }

  function onKey(ev: React.KeyboardEvent) {
    if (ev.key === 'ArrowDown') { ev.preventDefault(); setAt((i) => Math.min(i + 1, found.length - 1)) }
    else if (ev.key === 'ArrowUp') { ev.preventDefault(); setAt((i) => Math.max(i - 1, 0)) }
    else if (ev.key === 'Enter') { ev.preventDefault(); go(found[at]) }
    else if (ev.key === 'Escape') { ev.preventDefault(); onClose() }
  }

  return (
    <div className="fixed inset-0 z-100 grid items-start justify-items-center bg-ink/25 px-4 pt-[12vh]"
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}>
      <div role="dialog" aria-modal="true" aria-label="Jump to" data-testid="jump-to"
        className="w-full max-w-lg overflow-hidden rounded-lg border border-line bg-surface shadow-pop">
        <div className="flex items-center gap-2 border-b border-line px-3">
          <Icon name="search" className="size-4 text-ink-muted" />
          <input ref={input} value={typed} onChange={(e) => setTyped(e.currentTarget.value)}
            onKeyDown={onKey} role="combobox" aria-expanded="true" aria-controls="jump-list"
            aria-activedescendant={found[at] ? `jump-${found[at].key}` : undefined}
            aria-label="Jump to a report, dataset, schedule or page" data-testid="jump-input"
            placeholder="Jump to a report, dataset, schedule or page…"
            className="h-11 w-full bg-transparent text-body text-ink outline-none
                       placeholder:text-ink-muted" />
          <kbd className="rounded-sm border border-line px-1 text-micro text-ink-muted">Esc</kbd>
        </div>
        <ul id="jump-list" role="listbox" aria-label="Matches" className="max-h-80 overflow-y-auto p-1">
          {found.map((e, i) => (
            <li key={e.key} id={`jump-${e.key}`} role="option" aria-selected={i === at}
              data-testid="jump-option"
              onMouseMove={() => setAt(i)} onClick={() => go(e)}
              className={`flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-body ${
                i === at ? 'bg-hover text-ink' : 'text-ink'}`}>
              <span className="min-w-0 flex-1 truncate">{e.label}</span>
              <span className="text-caption text-ink-muted">{e.kind}</span>
            </li>
          ))}
          {found.length === 0 && (
            <li className="px-3 py-6 text-center text-small text-ink-muted">
              {catalog.live && catalog.isPending ? 'Loading…' : `Nothing called “${typed.trim()}”.`}
            </li>
          )}
        </ul>
      </div>
    </div>
  )
}
