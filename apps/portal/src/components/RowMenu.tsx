import { useEffect, useRef, useState } from 'react'
import { DeleteAction } from './DeleteAction'

/**
 * The actions a row has that are not opening it: today, deleting it.
 *
 * Behind ⋯ rather than spelled out beside Edit on every row. A list where
 * every line ends in "Edit Delete" is a list where Delete is always a few
 * pixels from the thing somebody meant to press. Choosing it here swaps the
 * menu for the same inline confirmation as before, so the question is still
 * asked beside the row it is about.
 */
export function RowMenu({ kind, name, label }: { kind: string; name: string; label: string }) {
  const [mode, setMode] = useState<'closed' | 'open' | 'deleting'>('closed')
  const box = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (mode !== 'open') return
    const away = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node)) setMode('closed')
    }
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setMode('closed') }
    document.addEventListener('mousedown', away)
    window.addEventListener('keydown', esc)
    return () => {
      document.removeEventListener('mousedown', away)
      window.removeEventListener('keydown', esc)
    }
  }, [mode])

  if (mode === 'deleting') {
    return <DeleteAction kind={kind} name={name} label={label} confirming
      onCancel={() => setMode('closed')} />
  }

  return (
    <div ref={box} className="relative">
      <button type="button" data-testid="row-menu" aria-haspopup="menu"
        aria-expanded={mode === 'open'} aria-label={`More actions for ${label}`}
        onClick={() => setMode((m) => (m === 'open' ? 'closed' : 'open'))}
        className="grid size-8 cursor-pointer place-items-center rounded-md text-ink-muted
                   hover:bg-hover hover:text-ink">
        <svg viewBox="0 0 24 24" className="size-4" fill="currentColor" aria-hidden>
          <circle cx="5" cy="12" r="1.75" /><circle cx="12" cy="12" r="1.75" />
          <circle cx="19" cy="12" r="1.75" />
        </svg>
      </button>
      {mode === 'open' && (
        <div role="menu" aria-label={`Actions for ${label}`}
          className="absolute top-9 right-0 z-30 grid w-40 rounded-lg border border-line
                     bg-surface p-1 shadow-pop">
          <button type="button" role="menuitem" data-testid="delete-action"
            onClick={() => setMode('deleting')}
            className="cursor-pointer rounded-md px-2.5 py-1.5 text-left text-body
                       text-delta-bad hover:bg-hover">
            Delete…
          </button>
        </div>
      )}
    </div>
  )
}
