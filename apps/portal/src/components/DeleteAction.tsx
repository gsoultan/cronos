import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ApiError, deleteDefinition } from '../lib/api'

/**
 * Removing a definition, and what stops it.
 *
 * Confirmed inline rather than in a dialog: the row already says which one it
 * is, and a modal repeating the name is a second place to read the same thing.
 *
 * The refusal is the interesting half. The server knows what points at this
 * and answers "invoices is still read by report \"billing\"" — which is a
 * sentence somebody can act on, and the reason this does not ask the question
 * itself. Two implementations of "what depends on what" is one more than can
 * be kept in step.
 */
export function DeleteAction({ kind, name, label }: {
  kind: string
  name: string
  /** What to call it in the confirmation. The title, not the identifier. */
  label?: string
}) {
  const [state, setState] = useState<'idle' | 'confirm' | 'busy'>('idle')
  const [refused, setRefused] = useState('')
  const queries = useQueryClient()

  async function go() {
    setState('busy')
    setRefused('')
    try {
      await deleteDefinition(kind, name)
      await queries.invalidateQueries()
    } catch (err) {
      setRefused(err instanceof ApiError ? err.message : 'Could not reach the server.')
      setState('idle')
    }
  }

  if (refused) {
    return (
      <span className="flex items-baseline gap-2">
        <span role="alert" data-testid="delete-refused" className="text-caption text-ink">
          {refused}
        </span>
        <button type="button" onClick={() => setRefused('')}
          className="shrink-0 cursor-pointer text-small text-ink-muted underline">Dismiss</button>
      </span>
    )
  }
  if (state === 'busy') {
    return <span className="text-small text-ink-muted">Deleting…</span>
  }
  if (state === 'confirm') {
    return (
      <span className="flex items-center gap-2 text-small">
        <span className="text-ink-secondary">Delete {label ?? name}?</span>
        <button type="button" onClick={go} data-testid="delete-confirm"
          className="cursor-pointer font-medium text-ink underline">Delete</button>
        <button type="button" onClick={() => setState('idle')}
          className="cursor-pointer text-ink-muted underline">Cancel</button>
      </span>
    )
  }
  return (
    <button type="button" onClick={() => setState('confirm')} data-testid="delete-action"
      className="cursor-pointer text-small text-ink-muted underline hover:text-ink">
      Delete
    </button>
  )
}
