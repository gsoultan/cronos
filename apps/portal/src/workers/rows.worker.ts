import { applyFilter } from '../lib/applyFilter'
import type { Field, Group } from '../lib/types'

/**
 * Row work, off the main thread.
 *
 * Filtering four thousand rows takes long enough to drop frames while someone
 * is typing into the filter builder, and the report page re-filters on every
 * change. AGENTS.md: the main thread is for painting.
 *
 * The rows are sent **once** and kept here. Posting them with every filter
 * would replace one main-thread cost with a structured-clone of the same size,
 * which is the usual way a worker makes things slower. Only the filter crosses
 * the boundary afterwards, and only a page of results comes back.
 */

interface LoadMessage {
  type: 'load'
  rows: Record<string, unknown>[]
  fields: Field[]
}

interface FilterMessage {
  type: 'filter'
  id: number
  filter: Group
  /** Aggregate these measures over the whole filtered set, not just the page. */
  sum?: string[]
  count?: { field: string; equals: string }[]
}

export type Incoming = LoadMessage | FilterMessage

export interface FilterResult {
  type: 'result'
  id: number
  total: number
  rows: Record<string, unknown>[]
  sums: Record<string, number>
  counts: Record<string, number>
  /** Milliseconds spent filtering. Kept fractional: rounding a fast filter
   *  to zero makes the timing vanish exactly when it is best. */
  ms: number
}

let rows: Record<string, unknown>[] = []
let fields: Field[] = []

/** Enough to fill the virtualiser's window and its overscan. */
const PAGE = 200

/* eslint-disable unicorn/require-post-message-target-origin --
   That rule targets window.postMessage. A second argument to
   Worker/DedicatedWorkerGlobalScope.postMessage is the *transfer list*, so
   taking the suggestion would silently try to transfer the origin string. */
self.addEventListener('message', (e: MessageEvent<Incoming>) => {
  const msg = e.data

  if (msg.type === 'load') {
    rows = msg.rows
    fields = msg.fields
    return
  }

  const started = performance.now()
  const matched = applyFilter(rows, msg.filter, fields)

  const sums: Record<string, number> = {}
  for (const field of msg.sum ?? []) {
    let total = 0
    for (const row of matched) total += Number(row[field]) || 0
    sums[field] = total
  }

  const counts: Record<string, number> = {}
  for (const { field, equals } of msg.count ?? []) {
    let n = 0
    for (const row of matched) if (row[field] === equals) n++
    counts[`${field}=${equals}`] = n
  }

  const result: FilterResult = {
    type: 'result',
    id: msg.id,
    total: matched.length,
    rows: matched.slice(0, PAGE),
    sums,
    counts,
    ms: Math.round((performance.now() - started) * 10) / 10,
  }
  self.postMessage(result)
})
